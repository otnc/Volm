// Copyright (C) 2026 otoneko.
// SPDX-License-Identifier: GPL-3.0-only

package main

import (
	"bytes"
	"fmt"
	"image"
	"image/png"
	"log"
	"math"
	"strings"
)

// A drawing is a list of shapes on a grid of width×height units, painted in order. The same list becomes both the SVG and the PNG files, so the two never differ.
type drawing struct {
	width, height float64
	shapes        []shape
}

// shape is a filled area or a stroke with round ends, in grid units.
type shape interface {
	// distance is the signed distance from (x, y) to the shape's edge, negative inside.
	distance(x, y float64) float64
	color() rgba
	svg() string
}

type rgba struct {
	r, g, b uint8
	a       float64
}

func hex(v uint32) rgba { return rgba{uint8(v >> 16), uint8(v >> 8), uint8(v), 1} }

func (c rgba) withAlpha(a float64) rgba { c.a = a; return c }

func (c rgba) svgPaint(attr string) string {
	s := fmt.Sprintf(`%s="#%02x%02x%02x"`, attr, c.r, c.g, c.b)
	if c.a < 1 {
		s += fmt.Sprintf(` %s-opacity="%.2f"`, attr, c.a)
	}
	return s
}

type roundRect struct {
	x, y, w, h, radius float64
	fill               rgba
}

func (s roundRect) distance(x, y float64) float64 {
	qx := math.Abs(x-(s.x+s.w/2)) - s.w/2 + s.radius
	qy := math.Abs(y-(s.y+s.h/2)) - s.h/2 + s.radius
	return math.Hypot(math.Max(qx, 0), math.Max(qy, 0)) + math.Min(math.Max(qx, qy), 0) - s.radius
}
func (s roundRect) color() rgba { return s.fill }
func (s roundRect) svg() string {
	return fmt.Sprintf(`<rect x="%g" y="%g" width="%g" height="%g" rx="%g" %s/>`, s.x, s.y, s.w, s.h, s.radius, s.fill.svgPaint("fill"))
}

type circle struct {
	cx, cy, r float64
	fill      rgba
}

func (s circle) distance(x, y float64) float64 { return math.Hypot(x-s.cx, y-s.cy) - s.r }
func (s circle) color() rgba                   { return s.fill }
func (s circle) svg() string {
	return fmt.Sprintf(`<circle cx="%g" cy="%g" r="%g" %s/>`, s.cx, s.cy, s.r, s.fill.svgPaint("fill"))
}

// line is a straight stroke with round ends.
type line struct {
	x1, y1, x2, y2, width float64
	stroke                rgba
}

func (s line) distance(x, y float64) float64 {
	px, py, dx, dy := x-s.x1, y-s.y1, s.x2-s.x1, s.y2-s.y1
	t := 0.0
	if l := dx*dx + dy*dy; l > 0 {
		t = math.Min(math.Max((px*dx+py*dy)/l, 0), 1)
	}
	return math.Hypot(px-dx*t, py-dy*t) - s.width/2
}
func (s line) color() rgba { return s.stroke }
func (s line) svg() string {
	return fmt.Sprintf(`<line x1="%.3f" y1="%.3f" x2="%.3f" y2="%.3f" stroke-width="%g" stroke-linecap="round" %s/>`, s.x1, s.y1, s.x2, s.y2, s.width, s.stroke.svgPaint("stroke"))
}

// arc is a stroke with round ends along a circle, from angle `from` to `to` in degrees, measured clockwise from 12 o'clock, with from < to.
type arc struct {
	cx, cy, r, from, to, width float64
	stroke                     rgba
}

// point returns the point of the arc's circle at angle deg.
func (s arc) point(deg float64) (x, y float64) {
	rad := deg * math.Pi / 180
	return s.cx + s.r*math.Sin(rad), s.cy - s.r*math.Cos(rad)
}

func (s arc) distance(x, y float64) float64 {
	deg := math.Atan2(x-s.cx, -(y-s.cy)) * 180 / math.Pi
	if deg >= s.from && deg <= s.to {
		return math.Abs(math.Hypot(x-s.cx, y-s.cy)-s.r) - s.width/2
	}
	ax, ay := s.point(s.from)
	bx, by := s.point(s.to)
	return math.Min(math.Hypot(x-ax, y-ay), math.Hypot(x-bx, y-by)) - s.width/2
}
func (s arc) color() rgba { return s.stroke }
func (s arc) svg() string {
	ax, ay := s.point(s.from)
	bx, by := s.point(s.to)
	large := 0
	if s.to-s.from > 180 {
		large = 1
	}
	return fmt.Sprintf(`<path d="M%.3f %.3fA%g %g 0 %d 1 %.3f %.3f" fill="none" stroke-width="%g" stroke-linecap="round" %s/>`,
		ax, ay, s.r, s.r, large, bx, by, s.width, s.stroke.svgPaint("stroke"))
}

func (d drawing) svg() []byte {
	var b strings.Builder
	fmt.Fprintf(&b, `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 %g %g" width="%g" height="%g">`+"\n", d.width, d.height, d.width*4, d.height*4)
	for _, s := range d.shapes {
		b.WriteString("  " + s.svg() + "\n")
	}
	b.WriteString("</svg>\n")
	return []byte(b.String())
}

// render rasterizes a square drawing at size×size pixels.
func (d drawing) render(size int) *image.NRGBA { return d.renderHeight(size) }

// renderHeight rasterizes the drawing at the given pixel height, keeping its aspect ratio. Each shape's edge fades over one pixel, which antialiases it without supersampling.
func (d drawing) renderHeight(h int) *image.NRGBA {
	scale := float64(h) / d.height
	w := int(math.Round(d.width * scale))
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	px := 1 / scale
	for y := range h {
		for x := range w {
			gx, gy := (float64(x)+0.5)*px, (float64(y)+0.5)*px
			var r, g, b, a float64 // premultiplied
			for _, s := range d.shapes {
				cover := math.Min(math.Max(0.5-s.distance(gx, gy)/px, 0), 1)
				if cover == 0 {
					continue
				}
				c := s.color()
				sa := c.a * cover
				r = float64(c.r)*sa + r*(1-sa)
				g = float64(c.g)*sa + g*(1-sa)
				b = float64(c.b)*sa + b*(1-sa)
				a = sa + a*(1-sa)
			}
			if a > 0 {
				i := img.PixOffset(x, y)
				img.Pix[i] = uint8(math.Round(r / a))
				img.Pix[i+1] = uint8(math.Round(g / a))
				img.Pix[i+2] = uint8(math.Round(b / a))
				img.Pix[i+3] = uint8(math.Round(a * 255))
			}
		}
	}
	return img
}

func encodePNG(img image.Image) []byte {
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		log.Fatal(err)
	}
	return buf.Bytes()
}

// polygon is a filled convex polygon whose points run clockwise on screen.
type polygon struct {
	points [][2]float64
	fill   rgba
}

func (s polygon) distance(x, y float64) float64 {
	// Largest signed distance to the edge lines: negative inside a convex polygon. Outside it overestimates near corners, which only softens the corner pixels.
	d := math.Inf(-1)
	for i, a := range s.points {
		b := s.points[(i+1)%len(s.points)]
		ex, ey := b[0]-a[0], b[1]-a[1]
		l := math.Hypot(ex, ey)
		d = math.Max(d, ((x-a[0])*ey-(y-a[1])*ex)/l)
	}
	return d
}
func (s polygon) color() rgba { return s.fill }
func (s polygon) svg() string {
	pts := make([]string, len(s.points))
	for i, p := range s.points {
		pts[i] = fmt.Sprintf("%g,%g", p[0], p[1])
	}
	return fmt.Sprintf(`<polygon points="%s" %s/>`, strings.Join(pts, " "), s.fill.svgPaint("fill"))
}
