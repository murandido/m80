# The distro is pinned with the Python minor (trixie is what 3.12-slim already resolved
# to), so a rebuild cannot silently move to another Debian release.
FROM python:3.12-slim-trixie

RUN apt-get update && apt-get install -y --no-install-recommends \
    libcairo2 \
    libpango-1.0-0 \
    libpangocairo-1.0-0 \
    libgdk-pixbuf-2.0-0 \
    libffi-dev \
    shared-mime-info \
    fonts-liberation \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app
COPY requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt
COPY app.py .

# Unprivileged, with a home for fontconfig's cache - the only thing WeasyPrint writes.
RUN useradd -m -u 10001 app
USER app

EXPOSE 5000
CMD ["python", "app.py"]
