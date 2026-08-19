FROM python:3.12-slim

WORKDIR /app

# 先装依赖，利用 Docker 层缓存
COPY requirements.txt .
RUN pip install --no-cache-dir -i https://pypi.tuna.tsinghua.edu.cn/simple -r requirements.txt

COPY . .

# 日志目录
RUN mkdir -p /app/logs

CMD ["python", "-m", "app.tasks"]
