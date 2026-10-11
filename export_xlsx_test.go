package main

import (
	"archive/zip"
	"encoding/xml"
	"image/png"
	"io"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteGroupLinksXLSXWithQR(t *testing.T) {
	t.Parallel()

	rows := []exportLinkRow{
		{Name: "香港节点", Link: "ss://YWVzLTEyOC1nY206cGFzc3dvcmQ@example.com:443#%E9%A6%99%E6%B8%AF%E8%8A%82%E7%82%B9"},
		{Name: "新加坡节点", Link: "socks5://user:password@203.0.113.8:1080#%E6%96%B0%E5%8A%A0%E5%9D%A1%E8%8A%82%E7%82%B9"},
	}
	path := filepath.Join(t.TempDir(), "节点导出.xlsx")
	if err := writeGroupLinksXLSXWithQR(path, rows); err != nil {
		t.Fatalf("生成 Excel 失败：%v", err)
	}

	archive, err := zip.OpenReader(path)
	if err != nil {
		t.Fatalf("打开 Excel 失败：%v", err)
	}
	defer archive.Close()

	files := make(map[string]*zip.File, len(archive.File))
	for _, file := range archive.File {
		files[file.Name] = file
		if strings.HasSuffix(file.Name, ".xml") || strings.HasSuffix(file.Name, ".rels") {
			validateExportXML(t, file)
		}
	}

	for _, name := range []string{
		"[Content_Types].xml",
		"xl/workbook.xml",
		"xl/worksheets/sheet1.xml",
		"xl/drawings/drawing1.xml",
		"xl/media/qr1.png",
		"xl/media/qr2.png",
	} {
		if files[name] == nil {
			t.Fatalf("Excel 缺少文件：%s", name)
		}
	}

	sheet := readExportZipFile(t, files["xl/worksheets/sheet1.xml"])
	for _, text := range []string{"节点名称", "协议链接", "节点二维码", rows[0].Name, rows[0].Link, rows[1].Name, rows[1].Link} {
		if !strings.Contains(sheet, text) {
			t.Fatalf("工作表缺少内容：%s", text)
		}
	}

	for _, name := range []string{"xl/media/qr1.png", "xl/media/qr2.png"} {
		reader, err := files[name].Open()
		if err != nil {
			t.Fatalf("打开二维码失败：%v", err)
		}
		config, decodeErr := png.DecodeConfig(reader)
		_ = reader.Close()
		if decodeErr != nil {
			t.Fatalf("二维码 PNG 无效：%v", decodeErr)
		}
		if config.Width < 100 || config.Height < 100 {
			t.Fatalf("二维码尺寸过小：%dx%d", config.Width, config.Height)
		}
	}
}

func validateExportXML(t *testing.T, file *zip.File) {
	t.Helper()
	reader, err := file.Open()
	if err != nil {
		t.Fatalf("打开 XML 失败：%v", err)
	}
	defer reader.Close()
	decoder := xml.NewDecoder(reader)
	for {
		if _, err := decoder.Token(); err != nil {
			if err == io.EOF {
				return
			}
			t.Fatalf("%s 不是有效 XML：%v", file.Name, err)
		}
	}
}

func readExportZipFile(t *testing.T, file *zip.File) string {
	t.Helper()
	reader, err := file.Open()
	if err != nil {
		t.Fatalf("打开 Excel 文件失败：%v", err)
	}
	defer reader.Close()
	data, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("读取 Excel 文件失败：%v", err)
	}
	return string(data)
}
