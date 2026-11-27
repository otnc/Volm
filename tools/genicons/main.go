// Copyright (C) 2026 otoneko.
// SPDX-License-Identifier: GPL-3.0-only

// Genicons draws Volm's icon and logo from the shapes defined in art.go and writes every file that uses them:
//
//	assets/icon.svg, assets/icon.png       the app icon; mygo build makes icon.png the exe icon
//	assets/volm.ico                        16 to 256 px, for the installer
//	assets/logo.svg                        the icon and the wordmark
//	internal/settings/appicon.png          128 px, for the settings window
//	internal/settings/logo.png             the logo at twice the size the settings window shows it
//
// Run it from the repository root after changing the artwork. With -preview, it also writes a sheet that shows the logo on light and dark backgrounds and the tray icon, which internal/trayicon draws, at its real sizes:
//
//	go run ./tools/genicons
//	go run ./tools/genicons -preview preview.png
package main

import (
	"bytes"
	"encoding/binary"
	"flag"
	"log"
	"os"
	"path/filepath"
)

// settingsLogoHeight is how tall the settings window shows the logo, in DIPs.
const settingsLogoHeight = 32

func main() {
	preview := flag.String("preview", "", "also write a review sheet of the artwork to this PNG path")
	flag.Parse()

	icon := iconArt()
	writeFile("assets/icon.svg", icon.svg())
	writeFile("assets/icon.png", encodePNG(icon.render(1024)))
	writeICO("assets/volm.ico", icon, []int{16, 20, 24, 32, 40, 48, 64, 128, 256})
	writeFile("internal/settings/appicon.png", encodePNG(icon.render(128)))

	logo := logoArt()
	writeFile("assets/logo.svg", logo.svg())
	writeFile("internal/settings/logo.png", encodePNG(logo.renderHeight(settingsLogoHeight*2)))

	if *preview != "" {
		writeFile(*preview, encodePNG(previewSheet()))
	}
}

// writeICO stores each size as PNG, which Windows Vista and later read from .ico files.
func writeICO(path string, d drawing, sizes []int) {
	var header, images bytes.Buffer
	must(binary.Write(&header, binary.LittleEndian, [3]uint16{0, 1, uint16(len(sizes))}))
	offset := 6 + 16*len(sizes)
	for _, size := range sizes {
		data := encodePNG(d.render(size))
		dim := uint8(size)
		if size >= 256 {
			dim = 0 // 0 means 256 in an icon directory entry
		}
		must(binary.Write(&header, binary.LittleEndian, struct {
			W, H, Colors, Reserved uint8
			Planes, BitCount       uint16
			Size, Offset           uint32
		}{dim, dim, 0, 0, 1, 32, uint32(len(data)), uint32(offset)}))
		images.Write(data)
		offset += len(data)
	}
	writeFile(path, append(header.Bytes(), images.Bytes()...))
}

func must(err error) {
	if err != nil {
		log.Fatal(err)
	}
}

func writeFile(path string, data []byte) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		log.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		log.Fatal(err)
	}
	log.Printf("wrote %s (%d bytes)", path, len(data))
}
