module github.com/Aadithya-J/code_nest/services/auth-service

go 1.24.4

require (
	github.com/Aadithya-J/code_nest/proto v0.0.0-00010101000000-000000000000
	github.com/go-gormigrate/gormigrate/v2 v2.1.5
	github.com/golang-jwt/jwt/v5 v5.2.1
	github.com/joho/godotenv v1.5.1
	github.com/stretchr/testify v1.11.1
	golang.org/x/crypto v0.41.0
	golang.org/x/oauth2 v0.30.0
	google.golang.org/grpc v1.75.0
	gopkg.in/square/go-jose.v2 v2.6.0
	gorm.io/driver/postgres v1.6.0
	gorm.io/gorm v1.30.1
)

require (
	cloud.google.com/go/compute/metadata v0.7.0 // indirect
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/jackc/pgpassfile v1.0.0 // indirect
	github.com/jackc/pgservicefile v0.0.0-20240606120523-5a60cdf6a761 // indirect
	github.com/jackc/pgx/v5 v5.7.5 // indirect
	github.com/jackc/puddle/v2 v2.2.2 // indirect
	github.com/jinzhu/inflection v1.0.0 // indirect
	github.com/jinzhu/now v1.1.5 // indirect
	github.com/kr/text v0.2.0 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	github.com/rogpeppe/go-internal v1.14.1 // indirect
	golang.org/x/net v0.43.0 // indirect
	golang.org/x/sync v0.16.0 // indirect
	golang.org/x/sys v0.35.0 // indirect
	golang.org/x/text v0.28.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20250826171959-ef028d996bc1 // indirect
	google.golang.org/protobuf v1.36.9 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

replace github.com/Aadithya-J/code_nest/proto => ../../proto
