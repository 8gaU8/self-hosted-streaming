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

CMD [".venv/bin/python", "app.py"]