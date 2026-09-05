FROM python:3.13-alpine

# uvバイナリを公式イメージからコピー(マルチステージの定石パターン)
COPY --from=ghcr.io/astral-sh/uv:latest /uv /uvx /bin/

WORKDIR /app

# 依存関係だけ先にコピーしてキャッシュを効かせる
COPY pyproject.toml uv.lock ./
RUN uv sync --frozen --no-dev

COPY app.py .
COPY templates/ templates/

EXPOSE 80

# --timeout is generous because proxied requests (large filebrowser
# downloads, Jellyfin streaming) can legitimately run long; gunicorn kills a
# worker that hasn't returned within this window.
CMD [".venv/bin/gunicorn", "--bind", "0.0.0.0:80", "--workers", "2", "--threads", "4", "--timeout", "300", "app:app"]