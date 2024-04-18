from typing import TYPE_CHECKING

from django.db import models
from django.db.models import Sum
from django.db.models.functions import Now
from django.utils.crypto import get_random_string
from django.utils.text import slugify
from django.utils.translation import gettext_lazy as _
from django_fsm import FSMField, transition

if TYPE_CHECKING:
    from django.db.models import Manager

DEFAULT_REPO = "github.com/ReviewSignal/k6-WordPress-benchmarks"


class BaseBareModel(models.Model):
    id = models.AutoField(primary_key=True, unique=True, editable=False)

    class Meta:  # pyright: ignore [reportIncompatibleVariableOverride]
        abstract = True


class BaseTimestampedModel(models.Model):
    created_at = models.DateTimeField(
        auto_now_add=True, verbose_name=_("Created at"), db_default=Now()
    )
    updated_at = models.DateTimeField(
        auto_now=True, verbose_name=_("Updated at"), db_default=Now()
    )

    class Meta:  # pyright: ignore [reportIncompatibleVariableOverride]
        abstract = True


class BaseNamedModel(BaseBareModel, BaseTimestampedModel):
    name = models.SlugField(unique=True)

    def __str__(self):
        return f"{self.name}"

    class Meta:  # pyright: ignore [reportIncompatibleVariableOverride]
        abstract = True


class TestLocation(BaseNamedModel):
    display_name = models.CharField(max_length=200)

    class Meta:  # pyright: ignore [reportIncompatibleVariableOverride]
        verbose_name = _("Test Location")
        verbose_name_plural = _("Test Locations")
        db_table = "loadtest_location"


class TestRun(BaseNamedModel):
    locations: 'Manager["TestRunLocation"]'

    target = models.URLField()

    source_repo = models.CharField(
        default=DEFAULT_REPO,
        max_length=200,
        verbose_name=_("Git repository"),
    )
    source_ref = models.CharField(
        default="main", max_length=200, verbose_name=_("Git Reference")
    )
    source_script = models.CharField(
        default="loadtest.js", max_length=200, verbose_name=_("Test script file")
    )

    @property
    def completed(self) -> bool:
        return self.pk and self.locations.exclude(status="completed").count() == 0

    @property
    def segments(self) -> list[str]:
        if not self.pk:
            return []

        locations = self.locations.all()
        workers = sum([location.num_workers for location in locations])

        return [f"{idx}/{workers}" for idx in range(1, workers + 1)]

    class Meta:  # pyright: ignore [reportIncompatibleVariableOverride]
        verbose_name = _("Test Run")
        verbose_name_plural = _("Test Runs")
        db_table = "loadtest_test_run"

    def save(self, *args, **kwargs):
        if not self.name:
            target = self.target.replace("https://", "").replace("http://", "")
            self.name = slugify(
                target.replace("/", "-").replace(".", "-")
                + "-"
                + get_random_string(5, "0123456789abcdefghijklmnopqrstuvwxyz")
            )
        super().save(*args, **kwargs)


class TestRunLocation(BaseBareModel):
    class State(models.TextChoices):
        PENDING = "pending", _("Pending")
        QUEUED = "queued", _("Queued")
        RUNNING = "running", _("Running")
        COMPLETED = "completed", _("Completed")
        FAILED = "failed", _("Failed")

    test_run = models.ForeignKey(
        TestRun,
        to_field="name",
        on_delete=models.CASCADE,
        related_name="locations",
        db_column="test_run",
    )
    location = models.ForeignKey(
        TestLocation,
        to_field="name",
        on_delete=models.PROTECT,
        related_name="+",
        db_column="location",
    )
    num_workers = models.PositiveSmallIntegerField(default=1)

    online_workers = models.PositiveSmallIntegerField(default=0)

    status = FSMField(default=State.PENDING, choices=State.choices)
    status_description = models.TextField(blank=True)

    @property
    def assigned_segments(self) -> list[str]:
        start = TestRunLocation.objects.filter(
            test_run=self.test_run, pk__lt=self.pk
        ).aggregate(Sum("num_workers"))
        total = TestRunLocation.objects.filter(test_run=self.test_run).aggregate(
            Sum("num_workers")
        )

        start = start.get("num_workers__sum", 0) or 0
        total = total.get("num_workers__sum", 0) or 0

        def get_segment_part(index: int, total: int) -> str:
            if index == 0:
                return "0"
            elif index == total:
                return "1"
            else:
                return f"{index}/{total}"

        return [
            f"{get_segment_part(idx-1, total)}:{get_segment_part(idx, total)}"
            for idx in range(start + 1, start + self.num_workers + 1)
        ]

    def get_status_description(self):
        if self.status_description:
            return self.status_description

        if self.status == State.PENDING:
            return _("Waiting for job to be accepted.")
        elif self.status == State.QUEUED:
            return _("Queued for execution. Waiting for workers to start come online.")
        elif self.status == State.RUNNING:
            return _("Test is running")
        elif self.status == State.COMPLETED:
            return _("Test has completed successfully")

    @transition(field=status, source=State.PENDING, target=State.QUEUED)
    def enqueue(self):
        pass

    @transition(field=status, source=State.QUEUED, target=State.RUNNING)
    def start(self):
        pass

    @transition(field=status, source=State.RUNNING, target=State.COMPLETED)
    def finish(self):
        pass

    @transition(field=status, target=State.FAILED)
    def fail(self, message: str | None = None):
        if message:
            self.status_description = message

    @transition(field=status, source=State.FAILED, target=State.PENDING)
    def retry(self, message: str | None = None):
        if message:
            self.status_description = message

    def __str__(self):
        return f"{self.test_run} - {self.location}"

    class Meta:  # pyright: ignore [reportIncompatibleVariableOverride]
        unique_together = ("test_run", "location")
        verbose_name = _("Test Run Location")
        verbose_name_plural = _("Test Run Locations")
        db_table = "loadtest_test_run_location"
