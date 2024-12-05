from urllib.parse import quote as urlquote

from dal import autocomplete
from django import forms
from django.contrib import admin
from django.db.models import QuerySet
from django.forms import ModelForm
from django.forms.models import BaseInlineFormSet
from django.forms.utils import flatatt
from django.http import HttpResponseRedirect
from django.urls import path, reverse
from django.utils.functional import cached_property
from django.utils.html import format_html
from django.utils.safestring import mark_safe
from django.utils.translation import gettext_lazy as _

# Register your models here.
from .models import (
    DEFAULT_REPO,
    TestLocation,
    TestOutputConfig,
    TestRun,
    TestRunEnvVar,
    TestRunLabel,
    TestRunLocation,
    duplicate_test_run,
)


class ReadOnlyWidget(forms.Widget):
    def __init__(self, attrs=None, value=None):
        super().__init__(attrs)
        self.value = value

    def render(self, name, value, attrs=None, renderer=None):
        final_attrs = self.build_attrs(
            attrs or {}, {"name": name, "value": self.value, "type": "hidden"}
        )
        return format_html("<input {}>{}", mark_safe(flatatt(final_attrs)), self.value)


class TestRunEnvVarForm(forms.ModelForm):
    class Meta:  # pyright: ignore reportIncompatibleVariableOverride
        model = TestRunEnvVar
        fields = "__all__"
        widgets = {
            "value": forms.Textarea(attrs={"rows": 2}),
        }


class TestRunEnvVarInline(admin.TabularInline):
    extra = 1
    form = TestRunEnvVarForm
    model = TestRunEnvVar

    def has_delete_permission(self, request, obj=None):
        return obj is not None and obj.draft

    def has_change_permission(self, request, obj=None):
        return obj is None or obj.draft

    def get_max_num(self, request, obj=None, **kwargs):
        return 0 if obj and not obj.draft else self.max_num


class TestRunLabelInline(admin.TabularInline):
    extra = 1
    model = TestRunLabel

    def has_delete_permission(self, request, obj=None):
        return obj is not None and obj.draft

    def has_change_permission(self, request, obj=None):
        return obj is None or obj.draft

    def get_max_num(self, request, obj=None, **kwargs):
        return 0 if obj and not obj.draft else self.max_num


class TestRunLocationForm(ModelForm):
    def __init__(self, *args, **kwargs):
        super().__init__(*args, **kwargs)
        self.fields["location"].widget.can_add_related = False
        self.fields["location"].widget.can_change_related = False
        self.fields["location"].widget.can_delete_related = False
        self.fields["location"].widget.can_view_related = False
        if self.instance.pk:
            self.fields["location"].widget = ReadOnlyWidget(
                value=self.instance.location
            )

    class Meta:  # pyright: ignore reportIncompatibleVariableOverride
        model = TestRunLocation
        fields = "__all__"


class TestRunLocationFormSet(BaseInlineFormSet):
    def __init__(self, *args, **kwargs):
        super().__init__(*args, **kwargs)
        locations = set(TestLocation.objects.values_list("name", flat=True))
        selected_locations = (
            {
                name
                for name in self.instance.locations.values_list("location", flat=True)
            }
            if self.instance and self.instance.pk
            else set()
        )
        available_locations = locations - selected_locations

        for form in self.forms:
            if not form.data and not form.instance.pk:
                location = available_locations.pop() if available_locations else None
                form.fields["location"].initial = location


class TestRunLocationInline(admin.TabularInline):
    model = TestRunLocation
    form = TestRunLocationForm
    formset = TestRunLocationFormSet
    readonly_fields = (
        "online_workers",
        "status",
        "status_description",
    )

    @cached_property
    def available_locations(self) -> list[TestLocation]:
        return list(TestLocation.objects.all())

    def get_extra(self, request, obj=None, **kwargs):
        if obj is None:
            return len(self.available_locations)
        return 0

    def get_max_num(self, request, obj=None, **kwargs):
        return len(self.available_locations)

    def has_delete_permission(self, request, obj=None):
        return obj is not None and obj.draft

    def has_change_permission(self, request, obj=None):
        return obj is None or obj.draft


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

    name = forms.CharField(required=False)

    class Meta:  # pyright: ignore reportIncompatibleVariableOverride
        model = TestRun
        fields = "__all__"


@admin.register(TestRun)
class TestRunAdmin(admin.ModelAdmin):
    list_display = [
        "name",
        "target",
        "results_link",
        "script",
        "completed",
        "status",
        "created_at",
        "started_at",
        "completed_at",
    ]
    date_hierarchy = "created_at"
    search_fields = ["target", "name", "source_repo", "source_script"]
    readonly_fields = (
        "draft",
        "name",
    )
    form = TestRunAdminForm
    inlines = [TestRunLocationInline, TestRunEnvVarInline, TestRunLabelInline]
    actions = ["start_test_runs", "cancel_test_runs"]

    def get_readonly_fields(self, request, obj=None):
        readonly_fields = list(super().get_readonly_fields(request, obj))
        if obj and not obj.draft:
            readonly_fields = [
                "name",
                "source_script",
                "source_ref",
                "target",
                "test_output",
                "resources_cpu",
                "resources_memory",
                "dedicated_nodes",
                "node_selector",
                "job_deadline",
            ] + readonly_fields
        return readonly_fields

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

    @admin.display(description="Results")
    def results_link(self, obj: TestRun):
        url = obj.grafana_url
        results_link = f'[ <a href="{url}" target="_blank">Summary</a> ]'
        return mark_safe(results_link)

    @admin.display(boolean=True)
    def completed(self, obj: TestRun):
        if obj.draft:
            return None

        status = obj.statuses
        total = obj.statuses["total"]

        if status.get(TestRunLocation.Status.FAILED, 0) > 0:
            return False
        elif status.get(TestRunLocation.Status.CANCELED, 0) > 0:
            return False
        elif status.get(TestRunLocation.Status.COMPLETED, 0) == total:
            return True
        return None

    @admin.action(
        description=f"Start selected {TestRun._meta.verbose_name_plural}",
        permissions=["change"],
    )
    def start_test_runs(self, request, queryset: QuerySet[TestRun]):
        count = 0
        for test_run in queryset.filter(draft=True).all():
            test_run.start()
            self.log_change(request, test_run, "Started test")
            count += 1

        self.message_user(
            request, f"Started {count} {TestRun._meta.verbose_name_plural}"
        )

    @admin.action(
        description=f"Cancel selected {TestRun._meta.verbose_name_plural}",
        permissions=["change"],
    )
    def cancel_test_runs(self, request, queryset: QuerySet[TestRun]):
        count = 0
        for test_run in queryset.filter(draft=False).all():
            status = test_run.statuses
            total = test_run.statuses["total"]
            if (
                status.get(TestRunLocation.Status.FAILED, 0) == 0
                and status.get(TestRunLocation.Status.COMPLETED, 0) != total
            ):
                count += 1
                test_run.cancel("Test manually canceled")
                self.log_change(request, test_run, "Canceled test")

        self.message_user(
            request, f"Canceled {count} {TestRun._meta.verbose_name_plural}"
        )

    def status(self, obj: TestRun):
        status = obj.statuses
        total = obj.statuses["total"]

        if obj.draft:
            return _("Draft")
        elif status.get(TestRunLocation.Status.FAILED, 0) > 0:
            return TestRunLocation.Status.FAILED.label
        elif status.get(TestRunLocation.Status.CANCELED, 0) > 0:
            return TestRunLocation.Status.CANCELED.label
        elif status.get(TestRunLocation.Status.COMPLETED, 0) == total:
            return TestRunLocation.Status.COMPLETED.label
        elif status.get(TestRunLocation.Status.PENDING, 0) > 0:
            return TestRunLocation.Status.PENDING.label
        elif status.get(TestRunLocation.Status.QUEUED, 0) > 0:
            return TestRunLocation.Status.QUEUED.label
        elif status.get(TestRunLocation.Status.READY, 0) > 0:
            return TestRunLocation.Status.READY.label
        elif status.get(TestRunLocation.Status.RUNNING, 0) > 0:
            return TestRunLocation.Status.RUNNING.label
        else:
            return _("Unknown")

    def changeform_view(self, request, object_id=None, form_url="", extra_context=None):
        obj: TestRun | None = self.get_object(request, object_id or "")
        extra_context = extra_context or {}
        extra_actions = {}

        if obj:
            extra_context["show_save_and_add_another"] = False

            extra_actions["duplicate"] = _("+ Duplicate")

            if obj.draft:
                extra_actions["saveandrun"] = _("▶ Save and Run test")

        extra_context["extra_actions"] = extra_actions  # pyright: ignore [reportArgumentType]

        return super().changeform_view(request, object_id, form_url, extra_context)

    def response_change(self, request, obj: TestRun):
        opts = self.opts
        msg_dict = {
            "name": opts.verbose_name,
            "obj": format_html('<a href="{}">{}</a>', urlquote(request.path), obj),
        }

        if "_saveandrun" in request.POST:
            msg = format_html(
                _("The {name} “{obj}” started successfully."),
                **msg_dict,
            )

            if obj:
                obj.draft = False
                obj.save()
                self.log_change(request, obj, "Started test")
                self.message_user(request, msg)

            return self.response_post_save_change(request, obj)

        if "_duplicate" in request.POST:
            msg = format_html(
                _("The {name} “{obj}” duplicated successfully. You can edit it below"),
                **msg_dict,
            )
            if obj:
                dup = duplicate_test_run(obj)
                self.log_addition(request, dup, f"Duplicated test {obj}")
                redirect_url = reverse(
                    "admin:%s_%s_change" % (dup._meta.app_label, dup._meta.model_name),
                    args=[dup.pk],
                )
                return HttpResponseRedirect(redirect_url)

        return super().response_change(request, obj)

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
    list_display = ["name", "display_name", "status", "last_ping"]
    prepopulated_fields = {"name": ["display_name"]}

    @admin.display(boolean=True)
    def status(self, obj: TestLocation):
        return obj.staus()


class TestOutputConfigForm(ModelForm):
    class Meta:  # pyright: ignore reportIncompatibleVariableOverride
        model = TestOutputConfig
        fields = "__all__"
        widgets = {
            "influxdb_token": forms.PasswordInput(
                attrs={"autocomplete": "off", "class": "vTextField"}
            ),
        }


@admin.register(TestOutputConfig)
class TestOutputConfigAdmin(admin.ModelAdmin):
    form = TestOutputConfigForm
    list_display = ["name", "influxdb_url", "influxdb_org", "influxdb_bucket"]
