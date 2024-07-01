from django.contrib.admin.apps import AdminConfig


class LoadtestingAdminConfig(AdminConfig):
    default_site = "webapp.admin.LoadtestingAdminSite"
