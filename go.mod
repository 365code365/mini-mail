module mail-server

go 1.23.12

require (
	github.com/mattn/go-sqlite3 v1.14.32
	github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common v1.1.54
	github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/dnspod v1.1.50
)

require github.com/gorilla/mux v1.8.1

require github.com/golang-jwt/jwt/v5 v5.3.0 // indirect
