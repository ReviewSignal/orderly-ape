from django.apps import AppConfig


class LoadtestConfig(AppConfig):
    default_auto_field = "django.db.models.BigAutoField"
    name = "loadtest"

    def ready(self):
        from . import signals  # noqa: F401
