from types import SimpleNamespace

from dal import autocomplete
from django import forms
from django.contrib import admin
from django.forms import ModelForm
from django.forms.models import BaseInlineFormSet
from django.forms.utils import flatatt
from django.urls import path, include
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
    readonly_fields = ("name",)
    form = TestRunAdminForm
    inlines = [TestRunLocationInline]

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
    prepopulated_fields = {"name": ["display_name"]}
