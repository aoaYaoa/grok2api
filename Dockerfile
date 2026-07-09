FROM python:3.13-slim

ENV PYTHONDONTWRITEBYTECODE=1 \
    PYTHONUNBUFFERED=1 \
    TZ=Asia/Shanghai \
    # 把 uv 包安装到系统 Python 环境
    UV_PROJECT_ENVIRONMENT=/opt/venv \
    PLAYWRIGHT_BROWSERS_PATH=/ms-playwright

# 确保 uv 的 bin 目录
ENV PATH="$UV_PROJECT_ENVIRONMENT/bin:$PATH"

RUN apt-get update \
    && apt-get install -y --no-install-recommends \
        tzdata \
        ca-certificates \
        curl \
        gnupg \
    && mkdir -p /etc/apt/keyrings \
    && curl -fsSL https://deb.nodesource.com/gpgkey/nodesource-repo.gpg.key \
        | gpg --dearmor -o /etc/apt/keyrings/nodesource.gpg \
    && echo "deb [signed-by=/etc/apt/keyrings/nodesource.gpg] https://deb.nodesource.com/node_20.x nodistro main" \
        > /etc/apt/sources.list.d/nodesource.list \
    && apt-get update \
    && apt-get install -y --no-install-recommends \
        nodejs \
        libasound2 \
        libatk-bridge2.0-0 \
        libatk1.0-0 \
        libc6 \
        libcairo2 \
        libcups2 \
        libdbus-1-3 \
        libdrm2 \
        libgbm1 \
        libglib2.0-0 \
        libgtk-3-0 \
        libnspr4 \
        libnss3 \
        libpango-1.0-0 \
        libx11-6 \
        libx11-xcb1 \
        libxcb1 \
        libxcomposite1 \
        libxdamage1 \
        libxext6 \
        libxfixes3 \
        libxkbcommon0 \
        libxrandr2 \
        xdg-utils \
        fonts-liberation \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app

# 安装 uv
COPY --from=ghcr.io/astral-sh/uv:latest /uv /uvx /bin/

COPY pyproject.toml uv.lock ./

RUN uv sync --frozen --no-dev --no-install-project

COPY config.defaults.toml ./
# 前端资源以预编译静态文件形式随 app 一起复制，镜像内不引入 Node 工具链
COPY app ./app
COPY main.py ./
COPY scripts ./scripts

RUN cd /app/app/services/browser_bridge \
    && npm install --omit=dev \
    && npx playwright install chromium

RUN mkdir -p /app/data /app/data/tmp /app/logs \
    && chmod +x /app/scripts/entrypoint.sh /app/scripts/init_storage.sh

EXPOSE 8000

ENTRYPOINT ["/app/scripts/entrypoint.sh"]

CMD ["uvicorn", "main:app", "--host", "0.0.0.0", "--port", "8000"]
