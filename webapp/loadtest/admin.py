from dal import autocomplete
from django import forms
from django.conf import settings
from django.contrib import admin
from django.db import models
from django.forms import ModelForm
from django.forms.models import BaseInlineFormSet
from django.forms.utils import flatatt
from django.urls import path
from django.utils.functional import cached_property
from django.utils.html import format_html
from django.utils.safestring import mark_safe

# Register your models here.
from .models import DEFAULT_REPO, TestLocation, TestRun, TestRunLocation


class ReadOnlyWidget(forms.Widget):
    def __init__(self, attrs=None, value=None):
        super().__init__(attrs)
        self.value = value

    def render(self, name, value, attrs=None, renderer=None):
        final_attrs = self.build_attrs(
            attrs or {}, {"name": name, "value": self.value, "type": "hidden"}
        )
        return format_html("<input {}>{}", mark_safe(flatatt(final_attrs)), self.value)


class TestRunLocationFormsSet(BaseInlineFormSet):
    def __init__(self, *args, **kwargs):
        super().__init__(*args, **kwargs)
        locations = iter(TestLocation.objects.values_list("name", flat=True))
        for form in self.forms:
            location = next(locations, None)
            if location is not None:
                form.fields["location"].widget = ReadOnlyWidget(value=location)


class TestRunLocationInline(admin.TabularInline):
    model = TestRunLocation
    formset = TestRunLocationFormsSet
    can_delete = False
    readonly_fields = ("online_workers", "status", "status_description")

    @cached_property
    def available_locations(self):
        return TestLocation.objects.values_list("name", flat=True)

    def get_extra(self, request, obj=None, **kwargs):
        if obj is None:
            return len(self.available_locations)
        return 0

    def get_max_num(self, request, obj=None, **kwargs):
        return self.get_extra(request, obj, **kwargs)

    def has_change_permission(self, request, obj=None):
        return False


def get_repo_choices():
    repos = TestRun.objects.values_list("source_repo", flat=True).distinct()
    if DEFAULT_REPO not in repos:
        repos = [DEFAULT_REPO] + list(repos)
    return repos


class TestRunRepositoryAutocompleteFromList(autocomplete.Select2ListView):
    def get_list(self) -> list[str]:
        return get_repo_choices()

    def create(self, text: str) -> str:
        return text


class TestRunAdminForm(ModelForm):
    def __init__(self, *args, **kwargs):
        super().__init__(*args, **kwargs)
        self.fields["source_repo"].choices = [
            (repo, repo) for repo in get_repo_choices()
        ]
        self.fields["source_repo"].initial = DEFAULT_REPO

    source_repo = autocomplete.Select2ListCreateChoiceField(
        widget=autocomplete.ListSelect2(url="admin:autocomplete_loadtest_source_repo"),
    )

    class Meta:  # pyright: ignore reportIncompatibleVariableOverride
        model = TestRun
        fields = "__all__"


@admin.register(TestRun)
class TestRunAdmin(admin.ModelAdmin):
    list_display = [
        "name",
        "target",
        "script",
        "live",
        "status",
        "created_at",
        "start_test_at",
    ]
    date_hierarchy = "created_at"
    search_fields = ["target", "name", "source_repo", "source_script"]
    readonly_fields = (
        "name",
        "draft",
    )
    form = TestRunAdminForm
    inlines = [TestRunLocationInline]

    @admin.display()
    def script(self, obj: TestRun):
        script = f"{obj.source_repo}/{obj.source_script}"
        if obj.source_ref != "main":
            script = f"{script}#{obj.source_ref}"

        if obj.source_repo.startswith("github.com"):
            link = (
                f"https://{obj.source_repo}/tree/{obj.source_ref}/{obj.source_script}"
            )
            script = f'<a href="{link}" target="_blank">{script}</a>'
        return mark_safe(script)

    @admin.display(boolean=True, ordering="draft")
    def live(self, obj: TestRun):
        return not obj.draft

    def status(self, obj: TestRun):
        status = obj.statuses
        total = obj.statuses["total"]

        if status.get(TestRunLocation.Status.FAILED, 0) > 0:
            return "failed"
        elif status.get(TestRunLocation.Status.COMPLETED, 0) == total:
            return "completed"
        elif status.get(TestRunLocation.Status.PENDING, 0) > 0:
            return "pending"
        elif status.get(TestRunLocation.Status.QUEUED, 0) > 0:
            return "queued"
        elif status.get(TestRunLocation.Status.READY, 0) > 0:
            return "ready"
        elif status.get(TestRunLocation.Status.RUNNING, 0) > 0:
            return "running"
        else:
            return "unknown"

    def changeform_view(self, request, object_id=None, form_url="", extra_context=None):
        extra_context = extra_context or {}

        # extra_context["show_save"] = False
        extra_context["show_save_and_continue"] = False

        return super().changeform_view(request, object_id, form_url, extra_context)

    def has_change_permission(self, request, obj=None):
        return getattr(settings, "DEBUG", False)

    def get_urls(self):
        autocomplete_urls = [
            path(
                "_autocomplete/source-repo",
                self.admin_site.admin_view(
                    TestRunRepositoryAutocompleteFromList.as_view()
                ),
                name="autocomplete_loadtest_source_repo",
            ),
        ]
        return autocomplete_urls + super().get_urls()


@admin.register(TestLocation)
class TestLocationAdmin(admin.ModelAdmin):
    list_display = ["name", "display_name"]
    prepopulated_fields = {"name": ["display_name"]}
