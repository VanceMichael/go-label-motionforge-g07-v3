# BENZHI_README

这是一个面向跨机器人数据采集项目的运营管理系统，用于协调采集场地与设备租约、多模态数据流、标注审核、数据集发布和训练任务。

## 标准构建、运行和测试命令

进入容器后执行：

```bash
# 编译
cd '/app' && GOTOOLCHAIN=local go build ./...

# 启动
cd '/app' && GOTOOLCHAIN=local go run ./cmd/server

# 测试
cd '/app' && GOTOOLCHAIN=local go test ./...
```

## Docker 构建和进入容器

```bash
chmod +x build_benzhi_docker.sh
./build_benzhi_docker.sh benzhi-task-312-amd64 linux/amd64
./build_benzhi_docker.sh benzhi-task-312-arm64 linux/arm64
docker run -it benzhi-task-312-amd64:latest
docker run -it --platform linux/arm64 benzhi-task-312-arm64:latest
```
