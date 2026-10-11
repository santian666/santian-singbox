package main

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"os"
	"strings"
)

type exportLinkRow struct {
	Name string
	Link string
}

func safeExportFileName(name string) string {
	text := strings.TrimSpace(name)
	if text == "" {
		text = "分组名称"
	}
	for _, item := range []string{"\\", "/", ":", "*", "?", "\"", "<", ">", "|"} {
		text = strings.ReplaceAll(text, item, "_")
	}
	if len([]rune(text)) > 40 {
		text = string([]rune(text)[:40])
	}
	return text
}

func writeGroupLinksXLSXWithQR(path string, rows []exportLinkRow) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()

	archive := zip.NewWriter(file)
	files := map[string]string{
		"[Content_Types].xml":                 xlsxQRContentTypes,
		"_rels/.rels":                         xlsxRootRels,
		"xl/_rels/workbook.xml.rels":          xlsxWorkbookRels,
		"xl/workbook.xml":                     xlsxWorkbook,
		"xl/styles.xml":                       xlsxStyles,
		"xl/worksheets/sheet1.xml":            buildLinkSheetWithQR(rows),
		"xl/worksheets/_rels/sheet1.xml.rels": xlsxSheetDrawingRels,
		"xl/drawings/drawing1.xml":            buildQRDrawing(rows),
		"xl/drawings/_rels/drawing1.xml.rels": buildQRDrawingRels(rows),
	}
	for name, content := range files {
		writer, err := archive.Create(name)
		if err != nil {
			_ = archive.Close()
			return err
		}
		if _, err := writer.Write([]byte(content)); err != nil {
			_ = archive.Close()
			return err
		}
	}
	for i, row := range rows {
		writer, err := archive.Create("xl/media/qr" + intText(i+1) + ".png")
		if err != nil {
			_ = archive.Close()
			return err
		}
		if err := writeQRPNG(writer, row.Link); err != nil {
			_ = archive.Close()
			return err
		}
	}
	return archive.Close()
}

func buildLinkSheetWithQR(rows []exportLinkRow) string {
	var buf bytes.Buffer
	buf.WriteString(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>`)
	buf.WriteString(`<worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships">`)
	endRow := len(rows) + 1
	buf.WriteString(`<dimension ref="A1:C`)
	buf.WriteString(intText(endRow))
	buf.WriteString(`"/><sheetViews><sheetView workbookViewId="0"/></sheetViews>`)
	buf.WriteString(`<sheetFormatPr defaultRowHeight="18"/>`)
	buf.WriteString(`<cols><col min="1" max="1" width="28" customWidth="1"/><col min="2" max="2" width="96" customWidth="1"/><col min="3" max="3" width="20" customWidth="1"/></cols>`)
	buf.WriteString(`<sheetData>`)
	writeXLSXRowWithHeight(&buf, 1, []string{"节点名称", "协议链接", "节点二维码"}, 18)
	for i, item := range rows {
		writeXLSXRowWithHeight(&buf, i+2, []string{item.Name, item.Link, ""}, 92)
	}
	buf.WriteString(`</sheetData><drawing r:id="rId1"/><pageMargins left="0.7" right="0.7" top="0.75" bottom="0.75" header="0.3" footer="0.3"/></worksheet>`)
	return buf.String()
}

func writeXLSXRowWithHeight(buf *bytes.Buffer, row int, values []string, height int) {
	buf.WriteString(`<row r="`)
	buf.WriteString(intText(row))
	if height > 0 {
		buf.WriteString(`" ht="`)
		buf.WriteString(intText(height))
		buf.WriteString(`" customHeight="1`)
	}
	buf.WriteString(`">`)
	for i, value := range values {
		col := string(rune('A' + i))
		buf.WriteString(`<c r="`)
		buf.WriteString(col)
		buf.WriteString(intText(row))
		buf.WriteString(`" t="inlineStr"><is><t>`)
		_ = xml.EscapeText(buf, []byte(value))
		buf.WriteString(`</t></is></c>`)
	}
	buf.WriteString(`</row>`)
}

func buildQRDrawing(rows []exportLinkRow) string {
	var buf bytes.Buffer
	buf.WriteString(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>`)
	buf.WriteString(`<xdr:wsDr xmlns:xdr="http://schemas.openxmlformats.org/drawingml/2006/spreadsheetDrawing" xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships">`)
	for i := range rows {
		id := i + 1
		buf.WriteString(`<xdr:oneCellAnchor><xdr:from><xdr:col>2</xdr:col><xdr:colOff>160000</xdr:colOff><xdr:row>`)
		buf.WriteString(intText(id))
		buf.WriteString(`</xdr:row><xdr:rowOff>70000</xdr:rowOff></xdr:from><xdr:ext cx="1047750" cy="1047750"/><xdr:pic><xdr:nvPicPr><xdr:cNvPr id="`)
		buf.WriteString(intText(id))
		buf.WriteString(`" name="QRCode `)
		buf.WriteString(intText(id))
		buf.WriteString(`"/><xdr:cNvPicPr><a:picLocks noChangeAspect="1"/></xdr:cNvPicPr></xdr:nvPicPr><xdr:blipFill><a:blip r:embed="rId`)
		buf.WriteString(intText(id))
		buf.WriteString(`"/><a:stretch><a:fillRect/></a:stretch></xdr:blipFill><xdr:spPr><a:prstGeom prst="rect"><a:avLst/></a:prstGeom></xdr:spPr></xdr:pic><xdr:clientData/></xdr:oneCellAnchor>`)
	}
	buf.WriteString(`</xdr:wsDr>`)
	return buf.String()
}

func buildQRDrawingRels(rows []exportLinkRow) string {
	var buf bytes.Buffer
	buf.WriteString(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?><Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">`)
	for i := range rows {
		id := i + 1
		buf.WriteString(`<Relationship Id="rId`)
		buf.WriteString(intText(id))
		buf.WriteString(`" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/image" Target="../media/qr`)
		buf.WriteString(intText(id))
		buf.WriteString(`.png"/>`)
	}
	buf.WriteString(`</Relationships>`)
	return buf.String()
}

func intText(value int) string {
	if value == 0 {
		return "0"
	}
	var digits [20]byte
	i := len(digits)
	for value > 0 {
		i--
		digits[i] = byte('0' + value%10)
		value /= 10
	}
	return string(digits[i:])
}

const xlsxQRContentTypes = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">
<Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>
<Default Extension="xml" ContentType="application/xml"/>
<Default Extension="png" ContentType="image/png"/>
<Override PartName="/xl/workbook.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.sheet.main+xml"/>
<Override PartName="/xl/worksheets/sheet1.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.worksheet+xml"/>
<Override PartName="/xl/drawings/drawing1.xml" ContentType="application/vnd.openxmlformats-officedocument.drawing+xml"/>
<Override PartName="/xl/styles.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.styles+xml"/>
</Types>`

const xlsxSheetDrawingRels = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/drawing" Target="../drawings/drawing1.xml"/>
</Relationships>`

const xlsxRootRels = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="xl/workbook.xml"/>
</Relationships>`

const xlsxWorkbookRels = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/worksheet" Target="worksheets/sheet1.xml"/>
<Relationship Id="rId2" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/styles" Target="styles.xml"/>
</Relationships>`

const xlsxWorkbook = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<workbook xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships">
<sheets><sheet name="节点" sheetId="1" r:id="rId1"/></sheets>
</workbook>`

const xlsxStyles = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<styleSheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main">
<fonts count="1"><font><sz val="11"/><name val="Microsoft YaHei UI"/></font></fonts>
<fills count="1"><fill><patternFill patternType="none"/></fill></fills>
<borders count="1"><border><left/><right/><top/><bottom/><diagonal/></border></borders>
<cellStyleXfs count="1"><xf numFmtId="0" fontId="0" fillId="0" borderId="0"/></cellStyleXfs>
<cellXfs count="1"><xf numFmtId="0" fontId="0" fillId="0" borderId="0" xfId="0"/></cellXfs>
</styleSheet>`
