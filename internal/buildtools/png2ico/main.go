package main

import (
	"encoding/binary"
	"flag"
	"fmt"
	"image/png"
	"os"
)

func main() {
	inPath := flag.String("in", "", "input PNG path")
	outPath := flag.String("out", "", "output ICO path")
	flag.Parse()

	if *inPath == "" || *outPath == "" {
		fmt.Fprintln(os.Stderr, "usage: png2ico -in <input.png> -out <output.ico>")
		os.Exit(2)
	}

	pngData, err := os.ReadFile(*inPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "read png: %v\n", err)
		os.Exit(1)
	}

	cfg, err := decodePNGConfig(*inPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "decode png config: %v\n", err)
		os.Exit(1)
	}
	if cfg.Width <= 0 || cfg.Height <= 0 || cfg.Width > 256 || cfg.Height > 256 {
		fmt.Fprintf(os.Stderr, "png dimensions must be 1..256, got %dx%d\n", cfg.Width, cfg.Height)
		os.Exit(1)
	}

	icoData, err := buildSingleIconICO(pngData, cfg.Width, cfg.Height)
	if err != nil {
		fmt.Fprintf(os.Stderr, "build ico: %v\n", err)
		os.Exit(1)
	}
	if err := os.WriteFile(*outPath, icoData, 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "write ico: %v\n", err)
		os.Exit(1)
	}
}

func decodePNGConfig(path string) (struct{ Width, Height int }, error) {
	file, err := os.Open(path)
	if err != nil {
		return struct{ Width, Height int }{}, err
	}
	defer file.Close()
	cfg, err := png.DecodeConfig(file)
	if err != nil {
		return struct{ Width, Height int }{}, err
	}
	return struct{ Width, Height int }{Width: cfg.Width, Height: cfg.Height}, nil
}

func buildSingleIconICO(pngData []byte, width int, height int) ([]byte, error) {
	const (
		iconDirSize      = 6
		iconDirEntrySize = 16
	)
	total := iconDirSize + iconDirEntrySize + len(pngData)
	buf := make([]byte, total)

	// ICONDIR
	binary.LittleEndian.PutUint16(buf[0:2], 0) // reserved
	binary.LittleEndian.PutUint16(buf[2:4], 1) // image type (icon)
	binary.LittleEndian.PutUint16(buf[4:6], 1) // image count

	entry := buf[6 : 6+16]
	entry[0] = iconDimByte(width)
	entry[1] = iconDimByte(height)
	entry[2] = 0                                  // palette
	entry[3] = 0                                  // reserved
	binary.LittleEndian.PutUint16(entry[4:6], 1)  // color planes
	binary.LittleEndian.PutUint16(entry[6:8], 32) // bits per pixel
	binary.LittleEndian.PutUint32(entry[8:12], uint32(len(pngData)))
	binary.LittleEndian.PutUint32(entry[12:16], uint32(iconDirSize+iconDirEntrySize))

	copy(buf[iconDirSize+iconDirEntrySize:], pngData)
	return buf, nil
}

func iconDimByte(v int) byte {
	if v >= 256 {
		return 0
	}
	return byte(v)
}
