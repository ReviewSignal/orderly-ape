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
        print(">>>> Setting start_test_at")
        test_run.start_test_at = timezone.now() + timezone.timedelta(minutes=5)
        test_run.save()
