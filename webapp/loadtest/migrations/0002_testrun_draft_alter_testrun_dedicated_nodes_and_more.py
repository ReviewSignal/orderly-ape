# Generated by Django 5.0.6 on 2024-07-01 22:18

from django.db import migrations, models


class Migration(migrations.Migration):

    dependencies = [
        ('loadtest', '0001_initial'),
    ]

    operations = [
        migrations.AddField(
            model_name='testrun',
            name='draft',
            field=models.BooleanField(default=True, verbose_name='Draft'),
        ),
        migrations.AlterField(
            model_name='testrun',
            name='dedicated_nodes',
            field=models.BooleanField(default=True, help_text="If enabled, each worker will run on a separate node (eg. separate VM). It's recommended to enable this option for more consistent results.", verbose_name='Run each worker on a separate node'),
        ),
        migrations.AlterField(
            model_name='testrun',
            name='job_deadline',
            field=models.CharField(default='1h', help_text='Time to allow workers to run. This should take into test fetching docker images, synctonization time, and actual test run time.', max_length=16, verbose_name='Job deadline'),
        ),
        migrations.AlterField(
            model_name='testrun',
            name='resources_cpu',
            field=models.CharField(default='1', help_text='Number of CPU cores to allocate for each worker.', max_length=16, verbose_name='Per-worker CPU'),
        ),
        migrations.AlterField(
            model_name='testrun',
            name='resources_memory',
            field=models.CharField(default='2G', help_text='Memory to allocate for each worker.', max_length=16, verbose_name='Per-worker memory'),
        ),
        migrations.AlterField(
            model_name='testrun',
            name='source_ref',
            field=models.CharField(default='main', help_text='Git reference to use when fetching the test script. It can be either a branch, a tag, or a commit hash.', max_length=200, verbose_name='Git Reference'),
        ),
        migrations.AlterField(
            model_name='testrun',
            name='source_repo',
            field=models.CharField(default='github.com/ReviewSignal/k6-WordPress-benchmarks', help_text='Git source repository to fetch the test script from.', max_length=200, verbose_name='Git repository'),
        ),
        migrations.AlterField(
            model_name='testrun',
            name='source_script',
            field=models.CharField(default='loadtest.js', help_text='Test script file, relative to the repository root.', max_length=200, verbose_name='Test script file'),
        ),
        migrations.AlterField(
            model_name='testrun',
            name='target',
            field=models.URLField(help_text='URL to test. It is passed to the test script as <code>TARGET</code> environment variable.'),
        ),
    ]
