.PHONY: build test lint vet

# Unit test only — integration 테스트는 제외 (//go:build integration 태그가 붙은 파일은 빌드되지 않음)
# CI(GitHub Actions)는 이 타겟만 실행한다.
test:
	go test ./...

# 전체 빌드 검증
build:
	go build ./...

# vet
vet:
	go vet ./...

# lint (golangci-lint 설치 필요)
lint:
	golangci-lint run ./...
