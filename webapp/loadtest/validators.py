import durationpy
from django.core.exceptions import ValidationError
from django.utils.translation import gettext_lazy as _


def validate_duration(value):
    try:
        durationpy.from_str(value)
    except durationpy.DurationError:
        raise ValidationError(
            _(
                "Invalid duration %(value)s. Use Golang duration format (https://pkg.go.dev/time#Duration)."
            ),
            params={"value": value},
        )
