from rest_framework import serializers

from .models import TestRun, TestRunLocation


class TestRunSerializer(serializers.ModelSerializer):
    completed = serializers.BooleanField(read_only=True)
    segments = serializers.ListField(read_only=True)

    class Meta:  # pyright: ignore [reportIncompatibleVariableOverride]
        model = TestRun
        fields = "__all__"


class JobSerializer(serializers.ModelSerializer):
    test_run = TestRunSerializer(read_only=True)
    assigned_segments = serializers.ListField(read_only=True)

    def validate_status(self, status):
        available_target_statuses = [
            t.target for t in self.instance.get_available_status_transitions()
        ]

        if self.instance and status not in available_target_statuses:
            raise serializers.ValidationError("Invalid status transition")

        return status

    def update(self, instance, validated_data):
        status = validated_data.get("status")
        if status:
            transitions = [
                t
                for t in instance.get_available_status_transitions()
                if t.target == status
            ]

            for t in transitions:
                t.method(instance)

            instance.save()
        return instance

    class Meta:  # pyright: ignore [reportIncompatibleVariableOverride]
        model = TestRunLocation
        fields = "__all__"
