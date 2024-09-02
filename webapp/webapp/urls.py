"""
URL configuration for webapp project.

The `urlpatterns` list routes URLs to views. For more information please see:
    https://docs.djangoproject.com/en/5.0/topics/http/urls/
Examples:
Function views
    1. Add an import:  from my_app import views
    2. Add a URL to urlpatterns:  path('', views.home, name='home')
Class-based views
    1. Add an import:  from other_app.views import Home
    2. Add a URL to urlpatterns:  path('', Home.as_view(), name='home')
Including another URLconf
    1. Import the include() function: from django.urls import include, path
    2. Add a URL to urlpatterns:  path('blog/', include('blog.urls'))
"""

from django.conf import settings
from django.conf.urls.static import static
from django.contrib import admin
from django.http import HttpResponse
from django.urls import include, path
from rest_framework import routers

from loadtest.views import PingViewSet, WorkersJobsViewSet


def ok(_):
    return HttpResponse("OK", content_type="text/plain")


router = routers.DefaultRouter(trailing_slash=False)
router.register("workers/(?P<location>[a-z0-9-]+)/jobs", WorkersJobsViewSet)
router.register("workers/(?P<location>[a-z0-9-]+)/ping", PingViewSet, basename="ping")

urlpatterns = [
    path("", ok, name="ok"),
    path("admin/", admin.site.urls),
    path("api-auth/", include("rest_framework.urls")),
    path("api/", include(router.urls)),
]

if settings.DEBUG:
    urlpatterns.extend(
        [
            path("__debug__/", include("debug_toolbar.urls")),
        ]
    )
    urlpatterns += static(settings.MEDIA_URL, document_root=settings.MEDIA_ROOT)
