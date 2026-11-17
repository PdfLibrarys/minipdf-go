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

使用方法については [Go ガイド](../README.md) をご覧ください。

## Visual results

- [XLSX](https://mini-software.github.io/minipdf-web-page/?language=go&suite=issue&format=xlsx)

## License

[Apache License 2.0](../LICENSE).
