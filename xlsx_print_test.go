package minipdf

import (
	"bytes"
	"image"
	"image/png"
	"strings"
	"testing"
)

func TestParseXLSXPrintRanges(t *testing.T) {
	tests := []struct {
		reference string
		want      []xlsxMerge
	}{
		{"'O''Brien, report'!$B$2:$D$80,'O''Brien, report'!$F$5", []xlsxMerge{
			{startRow: 2, endRow: 80, startColumn: 2, endColumn: 4},
			{startRow: 5, endRow: 5, startColumn: 6, endColumn: 6},
		}},
		{"Photos!$A:$Y", []xlsxMerge{{startRow: 1, endRow: 1048576, startColumn: 1, endColumn: 25}}},
		{"Photos!$2:$11", []xlsxMerge{{startRow: 2, endRow: 11, startColumn: 1, endColumn: 16384}}},
	}
	for _, test := range tests {
		t.Run(test.reference, func(t *testing.T) {
			got, err := parseXLSXPrintRanges(test.reference)
			if err != nil {
				t.Fatal(err)
			}
			if len(got) != len(test.want) {
				t.Fatalf("ranges = %#v, want %#v", got, test.want)
			}
			for index := range got {
				if got[index] != test.want[index] {
					t.Fatalf("ranges = %#v, want %#v", got, test.want)
				}
			}
		})
	}
	for _, reference := range []string{"", "'Unclosed!A1:B2", "Sheet!#REF!", "A0:B2", "B2:A1", "A1:XFE2", "A1:A1048577", "R1C1:R2C2"} {
		if _, err := parseXLSXPrintRanges(reference); err == nil {
			t.Fatalf("invalid print range %q was accepted", reference)
		}
	}
}

func TestConvertXLSXPrintAreaUsesRelationshipsAndPreservesSheetOrder(t *testing.T) {
	entries := map[string]string{
		"xl/workbook.xml": `<workbook xmlns:r="urn:relationships"><sheets>` +
			`<sheet name="Before" r:id="first"/><sheet name="Printed" r:id="print"/><sheet name="After" r:id="last"/>` +
			`</sheets><definedNames><definedName name="_xlnm.Print_Area" localSheetId="1">Printed!$B$2:$D$3</definedName></definedNames></workbook>`,
		"xl/_rels/workbook.xml.rels": `<Relationships>` +
			`<Relationship Id="first" Target="worksheets/sheet1.xml"/>` +
			`<Relationship Id="print" Target="/xl/worksheets/Sheet2.xml"/>` +
			`<Relationship Id="last" Target="worksheets/sheet3.xml"/></Relationships>`,
		"xl/worksheets/sheet1.xml": `<worksheet><sheetData><row r="1"><c r="A1" t="inlineStr"><is><t>Before</t></is></c></row></sheetData></worksheet>`,
		"xl/worksheets/sheet2.xml": `<worksheet><sheetData>` +
			`<row r="1"><c r="A1" t="inlineStr"><is><t>Outside top</t></is></c></row>` +
			`<row r="2"><c r="B2" t="inlineStr"><is><t>Printed</t></is></c><c r="E2" t="inlineStr"><is><t>Helper column</t></is></c></row>` +
			`<row r="500"><c r="B500" t="inlineStr"><is><t>Trailing data</t></is></c></row>` +
			`</sheetData></worksheet>`,
		"xl/worksheets/sheet3.xml": `<worksheet><sheetData><row r="1"><c r="A1" t="inlineStr"><is><t>After</t></is></c></row></sheetData></worksheet>`,
	}
	pdf, err := ConvertBytesToPDF(officePackageBytes(t, entries))
	assertPDFContains(t, pdf, err, "Before", "Printed", "After")
	for _, value := range []string{"Outside top", "Helper column", "Trailing data"} {
		if bytes.Contains(pdf, []byte(value)) {
			t.Fatalf("PDF contains out-of-print-area text %q", value)
		}
	}
	if pages := bytes.Count(pdf, []byte("/Type /Page ")); pages != 3 {
		t.Fatalf("pages = %d, want 3", pages)
	}
	if before, printed, after := bytes.Index(pdf, []byte("(Before)")), bytes.Index(pdf, []byte("(Printed)")), bytes.Index(pdf, []byte("(After)")); before >= printed || printed >= after {
		t.Fatal("mixed grid and text worksheets changed their order")
	}
}

func TestConvertXLSXPrintAreaRespectsHiddenCellsScaleAndRepeatedTitles(t *testing.T) {
	entries := map[string]string{
		"xl/workbook.xml": `<workbook><sheets><sheet name="Report"/></sheets><definedNames>` +
			`<definedName name="_xlnm.Print_Area" localSheetId="0">Report!$B$2:$D$80</definedName>` +
			`<definedName name="_xlnm.Print_Titles" localSheetId="0">Report!$2:$2</definedName>` +
			`</definedNames></workbook>`,
		"xl/worksheets/sheet1.xml": `<worksheet><dimension ref="A1:A1"/><cols>` +
			`<col min="2" max="4" width="40"/><col min="3" max="3" width="40" hidden="true"/>` +
			`</cols><sheetData>` +
			`<row r="2" ht="20"><c r="B2" t="inlineStr"><is><t>Header</t></is></c><c r="C2" t="inlineStr"><is><t>Hidden column</t></is></c></row>` +
			`<row r="3" ht="800" hidden="1"><c r="B3" t="inlineStr"><is><t>Hidden row</t></is></c></row>` +
			`<row r="70"><c r="B70" t="inlineStr"><is><t>First body</t></is></c></row>` +
			`<row r="71"><c r="B71" t="inlineStr"><is><t>Second body</t></is></c></row>` +
			`</sheetData><pageSetup scale="50" fitToHeight="1"/>` +
			`<rowBreaks><brk id="70" man="1"/></rowBreaks></worksheet>`,
	}
	pdf, err := ConvertBytesToPDF(officePackageBytes(t, entries))
	assertPDFContains(t, pdf, err, "Header", "First body", "Second body", "/F1 5.5 Tf")
	if pages := bytes.Count(pdf, []byte("/Type /Page ")); pages != 2 {
		t.Fatalf("pages = %d, want 2 scaled pages separated by the manual break", pages)
	}
	if count := bytes.Count(pdf, []byte("(Header)")); count != 2 {
		t.Fatalf("repeated header count = %d, want 2", count)
	}
	for _, text := range []string{"Hidden row", "Hidden column"} {
		if bytes.Contains(pdf, []byte(text)) {
			t.Fatalf("PDF contains %q", text)
		}
	}
}

func TestXLSXPrintBandsReserveTitleSpace(t *testing.T) {
	bands, err := xlsxPrintBands(xlsxRange{start: 2, end: 11}, 100, func(int) float64 { return 20 }, xlsxRange{start: 2, end: 3}, map[int]bool{6: true})
	if err != nil {
		t.Fatal(err)
	}
	want := []xlsxRange{{start: 2, end: 6}, {start: 7, end: 9}, {start: 10, end: 11}}
	if len(bands) != len(want) {
		t.Fatalf("bands = %#v, want %#v", bands, want)
	}
	for index := range bands {
		if bands[index] != want[index] {
			t.Fatalf("bands = %#v, want %#v", bands, want)
		}
	}
	if _, err := xlsxPrintBands(xlsxRange{start: 4, end: 10}, 30, func(int) float64 { return 20 }, xlsxRange{start: 1, end: 3}, nil); err == nil {
		t.Fatal("oversized repeated titles were accepted")
	}
}

func TestXLSXPrintFitModeOverridesPercentAndManualBreaks(t *testing.T) {
	grid, err := readXLSXGrid([]byte(`<worksheet><sheetPr><pageSetUpPr fitToPage="1"/></sheetPr>`+
		`<dimension ref="A1:D100"/><cols><col min="1" max="4" width="40"/></cols>`+
		`<sheetData><row r="1"><c r="A1" t="inlineStr"><is><t>Header</t></is></c></row></sheetData>`+
		`<pageSetup scale="400" fitToWidth="1" fitToHeight="1"/>`+
		`<rowBreaks><brk id="50" man="1"/></rowBreaks></worksheet>`), nil)
	if err != nil {
		t.Fatal(err)
	}
	document := NewPDFDocument()
	err = renderXLSXPrintGrid(document, grid, nil, xlsxPrintSettings{areas: []xlsxMerge{{startRow: 1, endRow: 100, startColumn: 1, endColumn: 4}}}, ConversionOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(document.pages) != 1 {
		t.Fatalf("fit-to-page pages = %d, want 1", len(document.pages))
	}
}

func TestConvertXLSXPrintAreaAllowsShapesWithoutDrawingRelationships(t *testing.T) {
	entries := map[string]string{
		"xl/workbook.xml": `<workbook><sheets><sheet name="Report"/></sheets><definedNames>` +
			`<definedName name="_xlnm.Print_Area" localSheetId="0">Report!A1:B2</definedName></definedNames></workbook>`,
		"xl/worksheets/sheet1.xml":            `<worksheet><sheetData><row r="1"><c r="A1" t="inlineStr"><is><t>Report</t></is></c></row></sheetData></worksheet>`,
		"xl/worksheets/_rels/sheet1.xml.rels": `<Relationships><Relationship Id="drawing" Type="urn:office/drawing" Target="../drawings/drawing1.xml"/></Relationships>`,
		"xl/drawings/drawing1.xml":            `<wsDr><twoCellAnchor><sp/></twoCellAnchor></wsDr>`,
	}
	pdf, err := ConvertBytesToPDF(officePackageBytes(t, entries))
	assertPDFContains(t, pdf, err, "Report")
}

func TestConvertXLSXRejectsInvalidPrintSettings(t *testing.T) {
	for _, value := range []string{"Report!#REF!", "Report!B2:A1"} {
		input := officePackageBytes(t, map[string]string{
			"xl/workbook.xml": `<workbook><sheets><sheet/></sheets><definedNames><definedName name="_xlnm.Print_Area" localSheetId="0">` +
				value + `</definedName></definedNames></workbook>`,
			"xl/worksheets/sheet1.xml": `<worksheet/>`,
		})
		_, err := ConvertBytesToPDF(input)
		assertPackageErrorContains(t, err, "Print_Area")
	}
	for _, setting := range []string{`scale="-1"`, `fitToWidth="-1"`} {
		_, err := readXLSXGrid([]byte(`<worksheet><pageSetup `+setting+`/></worksheet>`), nil)
		if err == nil || !strings.Contains(err.Error(), "invalid XLSX") {
			t.Fatalf("setting %s: error = %v", setting, err)
		}
	}
}

func TestXLSXPrintAreaRepeatsRowsAndColumns(t *testing.T) {
	grid, err := readXLSXGrid([]byte(`<worksheet><dimension ref="A1:F6"/>`+
		`<sheetData><row r="1" ht="100"><c r="A1" t="inlineStr"><is><t>Corner</t></is></c>`+
		`<c r="F1" t="inlineStr"><is><t>Row title</t></is></c></row>`+
		`<row r="6" ht="100"><c r="A6" t="inlineStr"><is><t>Column title</t></is></c></row></sheetData>`+
		`</worksheet>`), nil)
	if err != nil {
		t.Fatal(err)
	}
	grid.pageSize = PageSize{Width: 400, Height: 400}
	grid.margins = Margins{Left: 10, Right: 10, Top: 10, Bottom: 10}
	for index := 1; index <= 6; index++ {
		grid.columnWidths[index] = 100
		grid.rowHeights[index] = 100
	}
	document := NewPDFDocument()
	err = renderXLSXPrintGrid(document, grid, nil, xlsxPrintSettings{
		areas:   []xlsxMerge{{startRow: 1, endRow: 6, startColumn: 1, endColumn: 6}},
		rows:    xlsxRange{start: 1, end: 1},
		columns: xlsxRange{start: 1, end: 1},
	}, ConversionOptions{})
	if err != nil {
		t.Fatal(err)
	}
	pdf := document.Bytes()
	if len(document.pages) != 9 {
		t.Fatalf("pages = %d, want 9 with repeated rows and columns", len(document.pages))
	}
	for text, want := range map[string]int{"Corner": 9, "Row title": 3, "Column title": 3} {
		if count := bytes.Count(pdf, []byte("("+text+")")); count != want {
			t.Fatalf("%s count = %d, want %d", text, count, want)
		}
	}
}

func TestXLSXPrintAreaSupportsMultipleRangesAndOptionLimits(t *testing.T) {
	input := officePackageBytes(t, map[string]string{
		"xl/workbook.xml": `<workbook><sheets><sheet name="Report"/></sheets><definedNames>` +
			`<definedName name="_xlnm.Print_Area" localSheetId="0">Report!A1:B2,Report!D10:E11</definedName></definedNames></workbook>`,
		"xl/worksheets/sheet1.xml": `<worksheet><sheetData>` +
			`<row r="1"><c r="A1" t="inlineStr"><is><t>First range</t></is></c></row>` +
			`<row r="10"><c r="D10" t="inlineStr"><is><t>Second range</t></is></c></row></sheetData></worksheet>`,
	})
	for _, maxRows := range []int{0, 5} {
		pdf, err := ConvertBytesToPDFWithOptions(input, ConversionOptions{MaxRows: maxRows})
		assertPDFContains(t, pdf, err, "First range")
		wantPages := 2
		if maxRows > 0 {
			wantPages = 1
			if bytes.Contains(pdf, []byte("Second range")) {
				t.Fatal("MaxRows did not constrain the print area")
			}
		} else {
			assertPDFContains(t, pdf, nil, "Second range")
		}
		if pages := bytes.Count(pdf, []byte("/Type /Page ")); pages != wantPages {
			t.Fatalf("MaxRows=%d: pages = %d, want %d", maxRows, pages, wantPages)
		}
	}
}

func TestXLSXPrintAreaPaintsTextAfterAdjacentFills(t *testing.T) {
	grid, err := readXLSXGrid([]byte(`<worksheet><sheetData><row r="1">`+
		`<c r="A1" s="0" t="inlineStr"><is><t>Long report title</t></is></c><c r="B1" s="0"/>`+
		`</row></sheetData></worksheet>`), nil)
	if err != nil {
		t.Fatal(err)
	}
	fill := PDFColor{Red: 1}
	styles := []xlsxStyle{{font: xlsxFont{size: 11}, fill: &fill}}
	document := NewPDFDocument()
	err = renderXLSXPrintGrid(document, grid, styles, xlsxPrintSettings{areas: []xlsxMerge{{startRow: 1, endRow: 1, startColumn: 1, endColumn: 2}}}, ConversionOptions{})
	if err != nil {
		t.Fatal(err)
	}
	operations := document.pages[0].operations
	if _, ok := operations[len(operations)-1].(textOperation); !ok {
		t.Fatal("adjacent cell fill can paint over the overflowing report title")
	}
}

func TestConvertXLSXPrintsUsedAreaOfStyledSheetWithoutPrintArea(t *testing.T) {
	entries := map[string]string{
		"xl/workbook.xml": `<workbook><sheets><sheet name="Forecast"/></sheets></workbook>`,
		"xl/styles.xml": `<styleSheet><fonts><font><sz val="11"/></font></fonts>` +
			`<fills><fill><patternFill/></fill><fill><patternFill patternType="solid"><fgColor rgb="FF0000FF"/></patternFill></fill></fills>` +
			`<cellXfs><xf/><xf fillId="1"/><xf numFmtId="4"/></cellXfs></styleSheet>`,
		// The dimension reaches row 200 only through a cell whose style paints
		// nothing, and the extension list repeats a worksheet element name.
		"xl/worksheets/sheet1.xml": `<worksheet xmlns:x14="urn:x14"><dimension ref="A1:C200"/><sheetData>` +
			`<row r="1"><c r="A1" s="1" t="inlineStr"><is><t>Title</t></is></c></row>` +
			`<row r="2"><c r="B2" s="2"><f>B3*2</f><v>1234.5</v></c></row>` +
			`<row r="200"><c r="C200" s="0"/></row></sheetData>` +
			`<extLst><ext><x14:conditionalFormattings><x14:conditionalFormatting><x14:cfRule type="dataBar"/>` +
			`</x14:conditionalFormatting></x14:conditionalFormattings></ext></extLst></worksheet>`,
	}
	pdf, err := ConvertBytesToPDF(officePackageBytes(t, entries))
	assertPDFContains(t, pdf, err, "Title", "1,234.50", "0 0 1 rg")
	if count := bytes.Count(pdf, []byte("/Type /Page ")); count != 1 {
		t.Fatalf("pages = %d, want the used cells on 1 page", count)
	}
}

func TestConvertXLSXUsedAreaFallsBackToTextLayout(t *testing.T) {
	styles := `<styleSheet><fonts><font><sz val="11"/></font></fonts><cellXfs><xf/></cellXfs></styleSheet>`
	tests := map[string]string{
		"unsupported footer code": `<worksheet><sheetData><row r="1"><c r="A1" t="inlineStr"><is><t>Kept</t></is></c></row></sheetData>` +
			`<headerFooter><oddFooter>&amp;D</oddFooter></headerFooter></worksheet>`,
		"invalid conditional range": `<worksheet><sheetData><row r="1"><c r="A1" t="inlineStr"><is><t>Kept</t></is></c></row></sheetData>` +
			`<conditionalFormatting sqref="A:A"><cfRule type="expression" priority="1"><formula>TRUE</formula></cfRule></conditionalFormatting></worksheet>`,
	}
	for name, worksheet := range tests {
		t.Run(name, func(t *testing.T) {
			pdf, err := ConvertBytesToPDF(officePackageBytes(t, map[string]string{
				"xl/workbook.xml": `<workbook><sheets><sheet name="Data"/></sheets></workbook>`,
				"xl/styles.xml":   styles, "xl/worksheets/sheet1.xml": worksheet,
			}))
			assertPDFContains(t, pdf, err, "Kept")
			if count := bytes.Count(pdf, []byte("/Type /Page ")); count != 1 {
				t.Fatalf("pages = %d, want 1 text page and no partial grid pages", count)
			}
		})
	}
}

func TestConvertXLSXUsedAreaIncludesPictureBeyondCells(t *testing.T) {
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, image.NewRGBA(image.Rect(0, 0, 2, 2))); err != nil {
		t.Fatal(err)
	}
	pdf, err := ConvertBytesToPDF(officePackageBytes(t, map[string]string{
		"xl/workbook.xml": `<workbook><sheets><sheet name="Logo"/></sheets></workbook>`,
		"xl/styles.xml":   `<styleSheet><fonts><font><sz val="11"/></font></fonts><cellXfs><xf/></cellXfs></styleSheet>`,
		"xl/worksheets/sheet1.xml": `<worksheet><sheetData><row r="1"><c r="A1" t="inlineStr"><is><t>Title</t></is></c></row>` +
			`</sheetData></worksheet>`,
		"xl/worksheets/_rels/sheet1.xml.rels": `<Relationships><Relationship Type="urn:office/drawing" Target="../drawings/drawing1.xml"/></Relationships>`,
		// The picture fits inside D6, which no cell reaches.
		"xl/drawings/drawing1.xml": `<wsDr><oneCellAnchor><from><col>3</col><row>5</row></from><ext cx="127000" cy="127000"/>` +
			`<pic><blipFill><blip embed="image"/></blipFill></pic><clientData/></oneCellAnchor></wsDr>`,
		"xl/drawings/_rels/drawing1.xml.rels": `<Relationships><Relationship Id="image" Type="urn:office/image" Target="../media/logo.png"/></Relationships>`,
		"xl/media/logo.png":                   encoded.String(),
	}))
	assertPDFContains(t, pdf, err, "Title", "/ASCIIHexDecode /FlateDecode")
	if count := bytes.Count(pdf, []byte("/Type /Page ")); count != 1 {
		t.Fatalf("pages = %d, want 1", count)
	}
}

func TestReadXLSXGridPlacesRowsAndCellsWithoutReferences(t *testing.T) {
	grid, err := readXLSXGrid([]byte(`<worksheet><sheetData>`+
		`<row><c t="inlineStr"><is><t>A1</t></is></c><c t="inlineStr"><is><t>B1</t></is></c></row>`+
		`<row r="4" ht="30"><c r="C4" t="inlineStr"><is><t>C4</t></is></c><c t="inlineStr"><is><t>D4</t></is></c></row>`+
		`<row><c t="inlineStr"><is><t>A5</t></is></c></row></sheetData></worksheet>`), nil)
	if err != nil {
		t.Fatal(err)
	}
	for key, want := range map[[2]int]string{{1, 1}: "A1", {1, 2}: "B1", {4, 3}: "C4", {4, 4}: "D4", {5, 1}: "A5"} {
		if got := grid.cells[key].value; got != want {
			t.Errorf("cell %v = %q, want %q", key, got, want)
		}
	}
	if len(grid.cells) != 5 || grid.maxRow != 5 || grid.maxColumn != 4 || grid.rowHeights[4] != 30 {
		t.Fatalf("grid = %d cells, %d rows, %d columns, row heights %v", len(grid.cells), grid.maxRow, grid.maxColumn, grid.rowHeights)
	}
}

func TestXLSXGridExtendsForDrawingAnchorAndSpill(t *testing.T) {
	grid := xlsxGrid{maxRow: 1, maxColumn: 1, defaultRowHeight: 15, printScale: 1}
	columnWidth, rowHeight := grid.columnWidth(1), grid.rowHeight(1)
	// A drawing inside D6 adds only its own cell.
	grid.extendForAnchor(5, 3, 1, 1, columnWidth/2, rowHeight/2)
	if grid.maxRow != 6 || grid.maxColumn != 4 {
		t.Fatalf("anchor cell area = %d rows x %d columns, want 6 x 4", grid.maxRow, grid.maxColumn)
	}
	// A drawing that starts in D6 and is 2.5 cells in each direction reaches F8.
	grid.extendForAnchor(5, 3, 0, 0, columnWidth*2.5, rowHeight*2.5)
	if grid.maxRow != 8 || grid.maxColumn != 6 {
		t.Fatalf("spill area = %d rows x %d columns, want 8 x 6", grid.maxRow, grid.maxColumn)
	}
}

func TestXLSXUsedAreaCountsValuesFillsBordersAndMerges(t *testing.T) {
	fill := PDFColorBlack
	styles := []xlsxStyle{{}, {fill: &fill}, {border: xlsxBorder{bottom: 0.75}}}
	grid := xlsxGrid{maxRow: 90, maxColumn: 30, merges: []xlsxMerge{
		{startRow: 2, startColumn: 1, endRow: 2, endColumn: 6},
		{startRow: 40, startColumn: 1, endRow: 50, endColumn: 9},
	}, cells: map[[2]int]xlsxCell{
		{2, 1}:   {row: 2, column: 1, value: "Merged title"},
		{3, 4}:   {row: 3, column: 4, style: 1},
		{7, 2}:   {row: 7, column: 2, style: 2},
		{40, 1}:  {row: 40, column: 1, value: "  "},
		{90, 30}: {row: 90, column: 30},
	}}
	grid.trimToUsedArea(styles)
	if grid.maxRow != 7 || grid.maxColumn != 6 {
		t.Fatalf("used area = %d rows x %d columns, want 7 x 6", grid.maxRow, grid.maxColumn)
	}
	empty := xlsxGrid{maxRow: 5, maxColumn: 5, cells: map[[2]int]xlsxCell{{5, 5}: {row: 5, column: 5}}}
	empty.trimToUsedArea(styles)
	if empty.maxRow != 0 || empty.maxColumn != 0 {
		t.Fatalf("empty used area = %d x %d, want 0 x 0", empty.maxRow, empty.maxColumn)
	}
}

func TestVisibleXLSXWorksheetsGiveEverySheetNameAndDateSystem(t *testing.T) {
	files, err := openOfficePackage(officePackageBytes(t, map[string]string{
		"xl/workbook.xml": `<workbook><workbookPr date1904="1"/><sheets><sheet name="Plain"/><sheet name="Area"/></sheets><definedNames>` +
			`<definedName name="_xlnm.Print_Area" localSheetId="1">Area!$A$1:$B$2</definedName></definedNames></workbook>`,
		"xl/worksheets/sheet1.xml": `<worksheet><sheetData/></worksheet>`,
		"xl/worksheets/sheet2.xml": `<worksheet><sheetData/></worksheet>`,
	}))
	if err != nil {
		t.Fatal(err)
	}
	_, settings, err := visibleXLSXWorksheets(files, []string{"xl/worksheets/sheet1.xml", "xl/worksheets/sheet2.xml"})
	if err != nil {
		t.Fatal(err)
	}
	plain, area := settings["xl/worksheets/sheet1.xml"], settings["xl/worksheets/sheet2.xml"]
	if plain.sheetName != "Plain" || !plain.date1904 || len(plain.areas) != 0 {
		t.Fatalf("sheet without defined names = %+v", plain)
	}
	if area.sheetName != "Area" || !area.date1904 || len(area.areas) != 1 {
		t.Fatalf("sheet with a print area = %+v", area)
	}
}
