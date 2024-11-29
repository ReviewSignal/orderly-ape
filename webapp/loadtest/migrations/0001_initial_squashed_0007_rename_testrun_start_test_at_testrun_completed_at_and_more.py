# Generated by Django 5.1.2 on 2024-11-29 18:20

import django.db.models.deletion
import django.db.models.functions.datetime
import django_fsm
import loadtest.models
from django.db import migrations, models


# No need to RunPython for
# loadtest.migrations.0005_testoutputconfig_testrun_test_output
# as that is relevant only for existing installs.

class Migration(migrations.Migration):

    replaces = [('loadtest', '0001_initial'), ('loadtest', '0002_testrun_draft_alter_testrun_dedicated_nodes_and_more'), ('loadtest', '0003_testrunenvvar_testrunlabel'), ('loadtest', '0004_alter_testrunenvvar_value_and_more'), ('loadtest', '0005_testoutputconfig_testrun_test_output'), ('loadtest', '0006_testlocation_last_ping'), ('loadtest', '0007_rename_testrun_start_test_at_testrun_completed_at_and_more')]

    dependencies = [
    ]

    operations = [
        migrations.CreateModel(
            name='TestLocation',
            fields=[
                ('id', models.AutoField(editable=False, primary_key=True, serialize=False, unique=True)),
                ('created_at', models.DateTimeField(auto_now_add=True, db_default=django.db.models.functions.datetime.Now(), verbose_name='Created at')),
                ('updated_at', models.DateTimeField(auto_now=True, db_default=django.db.models.functions.datetime.Now(), verbose_name='Updated at')),
                ('name', models.SlugField(unique=True)),
                ('display_name', models.CharField(max_length=200)),
            ],
            options={
                'verbose_name': 'Test Location',
                'verbose_name_plural': 'Test Locations',
                'db_table': 'loadtest_location',
            },
        ),
        migrations.CreateModel(
            name='TestRun',
            fields=[
                ('id', models.AutoField(editable=False, primary_key=True, serialize=False, unique=True)),
                ('created_at', models.DateTimeField(auto_now_add=True, db_default=django.db.models.functions.datetime.Now(), verbose_name='Created at')),
                ('updated_at', models.DateTimeField(auto_now=True, db_default=django.db.models.functions.datetime.Now(), verbose_name='Updated at')),
                ('name', models.SlugField(unique=True)),
                ('target', models.URLField(help_text='URL to test. It is passed to the test script as <code>TARGET</code> environment variable.')),
                ('source_repo', models.CharField(default='github.com/ReviewSignal/k6-WordPress-benchmarks', help_text='Git source repository to fetch the test script from.', max_length=200, verbose_name='Git repository')),
                ('source_ref', models.CharField(default='main', help_text='Git reference to use when fetching the test script. It can be either a branch, a tag, or a commit hash.', max_length=200, verbose_name='Git Reference')),
                ('source_script', models.CharField(default='loadtest.js', help_text='Test script file, relative to the repository root.', max_length=200, verbose_name='Test script file')),
                ('start_test_at', models.DateTimeField(blank=True, editable=False, null=True, verbose_name='Start test at')),
                ('resources_cpu', models.CharField(default='1', help_text='Number of CPU cores to allocate for each worker.', max_length=16, verbose_name='Per-worker CPU')),
                ('resources_memory', models.CharField(default='2G', help_text='Memory to allocate for each worker.', max_length=16, verbose_name='Per-worker memory')),
                ('dedicated_nodes', models.BooleanField(default=True, help_text="If enabled, each worker will run on a separate node (eg. separate VM). It's recommended to enable this option for more consistent results.", verbose_name='Run each worker on a separate node')),
                ('node_selector', models.CharField(blank=True, help_text="Kubernetes node selector to use for worker pods (eg. 'cloud.google.com/gke-spot=true')", max_length=200, verbose_name='Node selector')),
                ('job_deadline', models.CharField(default='1h', help_text='Time to allow workers to run. This should take into test fetching docker images, synctonization time, and actual test run time.', max_length=16, verbose_name='Job deadline')),
                ('draft', models.BooleanField(default=True, verbose_name='Draft')),
            ],
            options={
                'verbose_name': 'Test Run',
                'verbose_name_plural': 'Test Runs',
                'db_table': 'loadtest_test_run',
            },
        ),
        migrations.CreateModel(
            name='TestRunLabel',
            fields=[
                ('id', models.AutoField(editable=False, primary_key=True, serialize=False, unique=True)),
                ('name', models.SlugField(help_text='Environment variable name', verbose_name='Name')),
                ('value', models.SlugField(help_text='Environment variable value', max_length=200, verbose_name='Value')),
                ('test_run', models.ForeignKey(db_column='test_run', on_delete=django.db.models.deletion.CASCADE, related_name='labels', to='loadtest.testrun', to_field='name')),
            ],
            options={
                'abstract': False,
            },
        ),
        migrations.CreateModel(
            name='TestRunEnvVar',
            fields=[
                ('id', models.AutoField(editable=False, primary_key=True, serialize=False, unique=True)),
                ('name', models.SlugField(help_text='Environment variable name', verbose_name='Name')),
                ('value', models.TextField(blank=True, help_text='Environment variable value', verbose_name='Value')),
                ('test_run', models.ForeignKey(db_column='test_run', on_delete=django.db.models.deletion.CASCADE, related_name='env_vars', to='loadtest.testrun', to_field='name')),
            ],
            options={
                'abstract': False,
            },
        ),
        migrations.CreateModel(
            name='TestRunLocation',
            fields=[
                ('id', models.AutoField(editable=False, primary_key=True, serialize=False, unique=True)),
                ('num_workers', models.PositiveSmallIntegerField(default=1)),
                ('online_workers', models.PositiveSmallIntegerField(default=0)),
                ('status', django_fsm.FSMField(choices=[('pending', 'Pending'), ('queued', 'Queued'), ('ready', 'Ready'), ('running', 'Running'), ('canceled', 'Canceled'), ('completed', 'Completed'), ('failed', 'Failed')], default='pending', max_length=50)),
                ('status_description', models.TextField(blank=True)),
                ('location', models.ForeignKey(db_column='location', on_delete=django.db.models.deletion.PROTECT, related_name='+', to='loadtest.testlocation', to_field='name')),
                ('test_run', models.ForeignKey(db_column='test_run', on_delete=django.db.models.deletion.CASCADE, related_name='locations', to='loadtest.testrun', to_field='name')),
            ],
            options={
                'verbose_name': 'Test Run Location',
                'verbose_name_plural': 'Test Run Locations',
                'db_table': 'loadtest_test_run_location',
                'unique_together': {('test_run', 'location')},
            },
        ),
        migrations.CreateModel(
            name='TestOutputConfig',
            fields=[
                ('id', models.AutoField(editable=False, primary_key=True, serialize=False, unique=True)),
                ('created_at', models.DateTimeField(auto_now_add=True, db_default=django.db.models.functions.datetime.Now(), verbose_name='Created at')),
                ('updated_at', models.DateTimeField(auto_now=True, db_default=django.db.models.functions.datetime.Now(), verbose_name='Updated at')),
                ('name', models.SlugField(unique=True)),
                ('influxdb_url', models.URLField(verbose_name='InfluxDB Server URL')),
                ('influxdb_token', models.CharField(max_length=200, verbose_name='InfluxDB Token')),
                ('influxdb_org', models.CharField(default='default', max_length=200, verbose_name='InfluxDB Organization')),
                ('influxdb_bucket', models.CharField(default='default', max_length=200, verbose_name='InfluxDB Bucket')),
                ('insecure_skip_verify', models.BooleanField(default=False, help_text='Use TLS but skip chain & host verification', verbose_name='Skip TLS verification')),
            ],
            options={
                'verbose_name': 'Test Output',
                'verbose_name_plural': 'Test Outputs',
                'db_table': 'loadtest_output',
            },
        ),
        migrations.AddField(
            model_name='testrun',
            name='test_output',
            field=models.ForeignKey(db_column='test_output', default=loadtest.models.TestOutputConfig.default, help_text='Influxdb configuration for storing test results.', on_delete=django.db.models.deletion.PROTECT, related_name='+', to='loadtest.testoutputconfig', to_field='name', verbose_name='Test Output'),
        ),
        migrations.AddField(
            model_name='testlocation',
            name='last_ping',
            field=models.DateTimeField(editable=False, null=True, verbose_name='Location last checkin'),
        ),
        migrations.RenameField(
            model_name='testrun',
            old_name='start_test_at',
            new_name='started_at',
        ),
        migrations.AlterField(
            model_name='testrun',
            name='started_at',
            field=models.DateTimeField(blank=True, editable=False, null=True, verbose_name='Test started time'),
        ),
        migrations.AddField(
            model_name='testrun',
            name='completed_at',
            field=models.DateTimeField(blank=True, editable=False, null=True, verbose_name='Test completed time'),
        ),
    ]
