# MiniPdf for Go

[Full guide (English)](../README.md)

## Quick start

```bash
go get github.com/PdfLibrarys/minipdf-go@latest
```

```go
import minipdf "github.com/PdfLibrarys/minipdf-go"

if err := minipdf.ConvertToPDF("report.docx", "report.pdf"); err != nil {
	panic(err)
}
```

사용 방법은 [Go 가이드](../README.md)를 참조하세요.

## Visual results

- [XLSX](https://mini-software.github.io/minipdf-web-page/?language=go&suite=issue&format=xlsx)

## License

[Apache License 2.0](../LICENSE).
