FROM python:3.12-slim-bookworm

RUN addgroup --system --gid 65532 nonroot && adduser --system --uid 65532 --gid 65532 --no-create-home --shell /sbin/nologin nonroot

ENV WEBROOT=/var/www/html
ENV PORT=8080

ENV PYTHONUNBUFFERED=1
ENV PYTHONDONTWRITEBYTECODE=1

RUN apt-get update && apt-get install -y --no-install-recommends \
    gcc gettext default-libmysqlclient-dev pkg-config \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /usr/src/app
COPY requirements.docker.txt ./requirements.txt
RUN pip install --no-cache-dir -r requirements.txt

COPY . .

RUN mkdir -p ${WEBROOT} \
    && rm -rf ${WEBROOT}/static \
    && SECRET_KEY=not-so-secret python manage.py collectstatic --noinput --clear -v2

USER nonroot:nonroot
CMD exec gunicorn webapp.wsgi:application -b 0.0.0.0:${PORT} -w $(( 2 * $(nproc) + 1 )) --name orderly-ape
