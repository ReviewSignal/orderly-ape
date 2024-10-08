from rest_framework import mixins, status, viewsets
from rest_framework.decorators import action
from rest_framework.response import Response

from .models import TestLocation, TestRunLocation
from .serializers import JobSerializer


class PingViewSet(viewsets.ViewSet):
    queryset = TestLocation.objects.all()

    def get_object(self):
        if "location" not in self.kwargs:
            return None

        return TestLocation.objects.filter(name=self.kwargs["location"]).first()

    def get_queryset(self):
        qs = self.queryset

        if "location" not in self.kwargs:
            return TestLocation.objects.none()

        if "location" in self.kwargs:
            qs = qs.filter(name=self.kwargs["location"])

        return qs

    def create(self, request, *args, **kwargs):
        instance = self.get_object()
        if instance:
            instance.ping()
            instance.save()
            return Response(status=status.HTTP_204_NO_CONTENT)
        return Response(status=status.HTTP_404_NOT_FOUND)


class WorkersJobsViewSet(
    mixins.ListModelMixin,
    mixins.RetrieveModelMixin,
    mixins.UpdateModelMixin,
    viewsets.GenericViewSet,
):
    serializer_class = JobSerializer
    queryset = TestRunLocation.objects.select_related("test_run").all()
    lookup_field = "test_run__name"

    def get_location(self):
        return self.kwargs.get("location", None)

    def get_queryset(self):
        name = self.kwargs.get(self.lookup_field)
        qs = self.queryset

        if "all" not in self.request.GET:
            qs = qs.filter(test_run__draft=False)

        if "location" not in self.kwargs:
            return TestRunLocation.objects.none()

        if name:
            qs = qs.filter(test_run__name=name)

        if "location" in self.kwargs:
            qs = qs.filter(location=self.kwargs["location"])

        return qs

    @action(detail=True, methods=["post"])
    def accept(self, request, *args, **kwargs):
        instance = self.get_object()
        instance.enqueue()
        instance.save()
        return Response(status=status.HTTP_200_OK)
