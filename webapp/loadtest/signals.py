from django.db.models.signals import post_save
from django.dispatch import receiver
from django.utils import timezone

from .models import TestRunLocation


@receiver(post_save, sender=TestRunLocation, dispatch_uid="set_start_test_at")
def set_start_test_at(sender, instance: TestRunLocation, created, **kwargs):
    test_run = instance.test_run
    if test_run is None:
        return

    if (
        instance.status == TestRunLocation.Status.READY
        and test_run.start_test_at is None
        and test_run.locations.exclude(status=TestRunLocation.Status.READY).count() == 0
    ):
        test_run.start_test_at = (
            timezone.now() + timezone.timedelta(seconds=20)
        ).replace(microsecond=0)
        test_run.save()


@receiver(post_save, sender=TestRunLocation, dispatch_uid="cancel_on_failure")
def cancel_on_failure(sender, instance: TestRunLocation, created, **kwargs):
    test_run = instance.test_run
    if test_run is None:
        return

    if instance.status == TestRunLocation.Status.FAILED:
        test_run.locations.exclude(
            status__in=[TestRunLocation.Status.FAILED, TestRunLocation.Status.COMPLETED]
        ).update(
            status=TestRunLocation.Status.CANCELED,
            status_description=f"Canceled automatically: Job was failing in {instance.location}.",
        )
