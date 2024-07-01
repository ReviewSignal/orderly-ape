from django.contrib import admin
from django.utils.translation import gettext_lazy as _


class LoadtestingAdminSite(admin.AdminSite):
    site_header = _("Orderly Ape")
    site_title = _("Orderly Ape Portal")
    index_title = _("Welcome to Orderly Ape Portal!")
