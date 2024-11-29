# Generated by Django 5.1 on 2024-09-16 11:19

from django.db import migrations, models


class Migration(migrations.Migration):
    dependencies = [
        ("loadtest", "0006_testlocation_last_ping"),
    ]

    operations = [
        migrations.RenameField(
            model_name="testrun",
            old_name="start_test_at",
            new_name="started_at",
        ),
        migrations.AlterField(
            model_name='testrun',
            name='started_at',
            field=models.DateTimeField(blank=True, editable=False, null=True, verbose_name='Test started time'),
        ),
        migrations.AddField(
            model_name="testrun",
            name="completed_at",
            field=models.DateTimeField(blank=True, editable=False, null=True, verbose_name="Test completed time"),
        ),
    ]