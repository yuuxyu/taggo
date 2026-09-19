# Linux では webkit2gtk-4.1 しか入っていない環境が一般的になったため、
# wails のビルドには毎回このタグが要る。忘れると 4.0 を探して失敗する。
WAILS_TAGS := webkit2_41

.PHONY: dev build test lint bindings clean

## dev: ホットリロード付きで起動する
dev:
	wails dev -tags $(WAILS_TAGS)

## build: デスクトップアプリを build/bin/taggo へ書き出す
build:
	wails build -tags $(WAILS_TAGS)

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
