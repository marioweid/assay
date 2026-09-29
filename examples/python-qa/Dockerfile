FROM ghcr.io/astral-sh/uv:0.12.10-python3.13-trixie-slim@sha256:3f30222c158072567236642664d80e232204d92f4912f2424d9fd5acdaa4f788

WORKDIR /workspace/examples/python-qa
ENV PYTHONDONTWRITEBYTECODE=1 PYTHONUNBUFFERED=1
COPY clients/python/assay/pyproject.toml clients/python/assay/README.md /workspace/clients/python/assay/
COPY clients/python/assay/src /workspace/clients/python/assay/src
COPY examples/python-qa/pyproject.toml examples/python-qa/uv.lock ./
RUN uv sync --frozen --no-dev
COPY examples/python-qa/app.py examples/python-qa/server.py examples/python-qa/chat_sessions.py ./
COPY examples/python-qa/static ./static
USER 10001:10001
EXPOSE 8090
CMD ["/workspace/examples/python-qa/.venv/bin/uvicorn", "server:app", "--host", "0.0.0.0", "--port", "8090"]
