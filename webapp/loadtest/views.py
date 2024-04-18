from rest_framework import mixins, status, viewsets
from rest_framework.decorators import action
from rest_framework.response import Response

from .models import TestRun, TestRunLocation
from .serializers import JobSerializer


class WorkersJobsViewSet(
    mixins.ListModelMixin,
    mixins.RetrieveModelMixin,
    mixins.UpdateModelMixin,
    viewsets.GenericViewSet,
):
    queryset = TestRunLocation.objects.none()
    serializer_class = JobSerializer

    def get_location(self):
        return self.kwargs.get("location", None)

    def get_queryset(self):
        queryset = TestRunLocation.objects
        if "location" not in self.kwargs:
            return queryset.none()

        if "location" in self.kwargs:
            queryset = queryset.filter(location=self.kwargs["location"])

        # if self.request.user.is_authenticated:
        #     queryset = queryset.filter(user=self.request.user)
        return queryset

    @action(detail=True, methods=["post"])
    def accept(self, request, *args, **kwargs):
        instance = self.get_object()
        instance.enqueue()
        instance.save()
        return Response(status=status.HTTP_200_OK)
