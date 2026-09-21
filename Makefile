.PHONY: dev build test lint bindings clean

## dev: ホットリロード付きで起動する
dev:
	wails dev

## build: デスクトップアプリを build/bin/taggo.exe へ書き出す
build:
	wails build

## test: Go 側のテストを実行する
test:
	go test ./...

## lint: 整形と静的解析を確認する
lint:
	gofmt -l . | grep -v node_modules | grep -v frontend/dist; go vet ./...
	cd frontend && npx tsc --noEmit

## bindings: Go の公開 API から TypeScript のバインディングを作り直す
bindings:
	wails generate module

## clean: ビルド成果物を消す
clean:
	rm -rf build/bin frontend/dist
