from rest_framework import serializers
from rest_framework.reverse import reverse

from .models import TestRun, TestRunLocation


class TestRunSerializer(serializers.ModelSerializer):
    completed = serializers.BooleanField(read_only=True)
    ready = serializers.BooleanField(read_only=True)
    segments = serializers.ListField(read_only=True)

    class Meta:  # pyright: ignore [reportIncompatibleVariableOverride]
        model = TestRun
        exclude = ["id", "name"]


class JobSerializer(serializers.HyperlinkedModelSerializer):
    name = serializers.CharField(source="test_run.name", read_only=True)
    url = serializers.SerializerMethodField()
    test_run = TestRunSerializer(read_only=True)
    assigned_segments = serializers.ListField(read_only=True)

    def get_url(self, obj):
        return reverse(
            "testrunlocation-detail",
            kwargs={"location": obj.location, "test_run__name": obj.test_run.name},
            request=self.context.get("request"),
        )

    def validate_status(self, status):
        if self.instance and status == self.instance.status:
            return status

        available_target_statuses = [
            t.target for t in self.instance.get_available_status_transitions()
        ]

        if self.instance and status not in available_target_statuses:
            raise serializers.ValidationError(
                f"Invalid status transition from `{self.instance.status}` to `{status}`"
            )

        return status

    def update(self, instance: TestRunLocation, validated_data: dict):
        status = validated_data.get("status")

        if status and instance.status != status:
            transitions = [
                t
                for t in instance.get_available_status_transitions()  # pyright: ignore [reportAttributeAccessIssue]
                if t.target == status
            ]

            for t in transitions:
                t.method(instance)

        return super().update(instance, validated_data)

    class Meta:  # pyright: ignore [reportIncompatibleVariableOverride]
        model = TestRunLocation
        fields = "__all__"
