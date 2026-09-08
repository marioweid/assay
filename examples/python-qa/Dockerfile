FROM ghcr.io/astral-sh/uv:0.12.10-python3.13-trixie-slim@sha256:3f30222c158072567236642664d80e232204d92f4912f2424d9fd5acdaa4f788

WORKDIR /app
ENV PYTHONDONTWRITEBYTECODE=1 PYTHONUNBUFFERED=1
COPY pyproject.toml uv.lock ./
RUN uv sync --frozen --no-dev
COPY app.py server.py ./
COPY static ./static
USER 10001:10001
EXPOSE 8090
CMD ["/app/.venv/bin/uvicorn", "server:app", "--host", "0.0.0.0", "--port", "8090"]
