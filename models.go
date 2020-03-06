package main

/*
type Palette struct {
	PaletteID uint64 `db:"palette_id"`
	Order uint8
	HSVID uint64 `db:"hsv_id"`
	RGBID uint64 `db:"rgb_id"`
}
var paletter_schema = `
CREATE TABLE hsv (
	palette_id INT NOT NULL,
	order UNSIGNED INT NOT NULL COMMENT 'order for the list of colors associated with the palette_id',
	hsv_id UNSIGNED BIGINT NOT NULL,
	rgb_id UNSIGNED BIGINT NOT NULL,
)ENGINE=INNODB
`

type HSV struct {
	ID uint64
	Hue uint16 // [0-360)
	Saturation uint64 // [0-1]
	Value uint64 // [0-1]
}
var hsv_schema = `
CREATE TABLE hsv (
	id UNSIGNED BIGINT PRIMARY KEY AUTO_INCREMENT,
	hue UNSIGNED INT NOT NULL COMMENT 'only store values [0-360)',
	saturation UNSIGNED FLOAT NOT NULL COMMENT 'only store values [0-1]',
	value UNSIGNED FLOAT NOT NULL COMMENT 'only store values [0-1]',
	UNIQUE INDEX hsv (hue, saturation, value)
)ENGINE=INNODB
`

type RGB struct {
	ID uint64
	Red uint8 // [0-255]
	Green uint8 // [0-255]
	Blue uint8 // [0-255]
}
var rgb_schema = `
CREATE TABLE hsv (
	id UNSIGNED BIGINT PRIMARY KEY AUTO_INCREMENT,
	red UNSIGNED INT NOT NULL COMMENT 'only store values [0-255]',
	green UNSIGNED INT NOT NULL COMMENT 'only store values [0-255]',
	blue UNSIGNED INT NOT NULL COMMENT 'only store values [0-255]',
	UNIQUE INDEX rgb (red, green, blue)
)ENGINE=INNODB
`

type Escape struct {
	X float64
	Y float64
	Magnification float64
	Escape uint16
}
var escape_schema = `
CREATE TABLE escape (
	x DOUBLE NOT NULL,
	y DOUBLE NOT NULL,
	magnification DOUBLE NOT NULL,
	escape UNSIGNED INT NOT NULL COMMENT 'The number of iterations it took to escape the boundary ',
	UNIQUE INDEX xymi (x, y, magnification)
)ENGINE=INNODB
`
*/
