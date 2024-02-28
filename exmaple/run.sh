 dapr run --app-id inf-dapr-sdk-demo --dapr-http-port 3500 --app-protocol grpc --app-port 2000 -- go run main.go

# 启动的是HTTP服务
#dapr run --app-id inf-dapr-sdk-demo --dapr-http-port 3500 --app-protocol http --app-port 2000 -- go run main.go

