package web

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"io"
	"math"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var statsigStyleNumberPattern = regexp.MustCompile(`[\d.\-]+`)

type statsigMaterials struct {
	VerificationToken string
	SVGData           string
	Indexes           []int
}

func localStatsigSignature(method, path string, materials statsigMaterials, now time.Time, random io.Reader) (string, error) {
	fingerprint, err := base64.StdEncoding.DecodeString(strings.TrimSpace(materials.VerificationToken))
	if err != nil {
		fingerprint, err = base64.RawStdEncoding.DecodeString(strings.TrimSpace(materials.VerificationToken))
	}
	if err != nil || len(fingerprint) != 48 {
		return "", fmt.Errorf("Statsig verification token 格式无效")
	}
	animationFingerprint, err := statsigAnimationFingerprint(fingerprint, materials.SVGData, materials.Indexes)
	if err != nil {
		return "", err
	}
	if random == nil {
		random = rand.Reader
	}
	method = strings.ToUpper(strings.TrimSpace(method))
	if method == "" {
		method = http.MethodPost
	}
	if path == "" {
		path = "/"
	}
	counter := now.Unix() - defaultStatsigEpoch
	if counter < 0 {
		counter = 0
	}
	digest := sha256.Sum256([]byte(method + "!" + path + "!" + strconv.FormatInt(counter, 10) + "obfiowerehiring" + animationFingerprint))
	mask := []byte{0}
	if _, err := io.ReadFull(random, mask); err != nil {
		return "", fmt.Errorf("生成 Statsig 掩码: %w", err)
	}

	payload := make([]byte, 0, 69)
	payload = append(payload, fingerprint...)
	var littleEndianCounter [4]byte
	binary.LittleEndian.PutUint32(littleEndianCounter[:], uint32(counter))
	payload = append(payload, littleEndianCounter[:]...)
	payload = append(payload, digest[:16]...)
	payload = append(payload, defaultStatsigTrailer)

	encoded := make([]byte, 1, 70)
	encoded[0] = mask[0]
	for _, value := range payload {
		encoded = append(encoded, value^mask[0])
	}
	return base64.RawStdEncoding.EncodeToString(encoded), nil
}

func statsigAnimationFingerprint(fingerprint []byte, svg string, indexes []int) (string, error) {
	if len(fingerprint) != 48 || len(indexes) < 4 {
		return "", fmt.Errorf("Statsig 动画索引无效")
	}
	for _, index := range indexes[:4] {
		if index < 0 || index >= len(fingerprint) {
			return "", fmt.Errorf("Statsig 动画索引越界")
		}
	}
	paths, err := parseStatsigSVG(svg)
	if err != nil {
		return "", err
	}
	selected := int(fingerprint[indexes[0]] % 16)
	if selected >= len(paths) {
		return "", fmt.Errorf("Statsig 动画帧缺失")
	}
	c := int(fingerprint[indexes[1]]%16) * int(fingerprint[indexes[2]]%16) * int(fingerprint[indexes[3]]%16)
	color, transform, err := simulateStatsigStyle(paths[selected], c)
	if err != nil {
		return "", err
	}
	matches := statsigStyleNumberPattern.FindAllString(color+transform, -1)
	converted := make([]string, 0, len(matches))
	for _, match := range matches {
		value, err := strconv.ParseFloat(match, 64)
		if err != nil {
			return "", fmt.Errorf("解析 Statsig 动画数值: %w", err)
		}
		converted = append(converted, statsigNumberHex(value))
	}
	return strings.NewReplacer(".", "", "-", "").Replace(strings.Join(converted, "")), nil
}

func parseStatsigSVG(svg string) ([][]int, error) {
	if len(svg) < 9 {
		return nil, fmt.Errorf("Statsig SVG 数据无效")
	}
	parts := strings.Split(svg[9:], "C")
	result := make([][]int, 0, len(parts))
	nonDigit := regexp.MustCompile(`[^\d]+`)
	for _, part := range parts {
		cleaned := strings.TrimSpace(nonDigit.ReplaceAllString(part, " "))
		if cleaned == "" {
			result = append(result, []int{0})
			continue
		}
		fields := strings.Fields(cleaned)
		values := make([]int, 0, len(fields))
		for _, field := range fields {
			value, err := strconv.Atoi(field)
			if err != nil {
				return nil, fmt.Errorf("解析 Statsig SVG: %w", err)
			}
			values = append(values, value)
		}
		result = append(result, values)
	}
	return result, nil
}

func statsigH(value, parameter, target float64, floorValue bool) float64 {
	result := value*(target-parameter)/255 + parameter
	if floorValue {
		return math.Floor(result)
	}
	result = math.Round(result*100) / 100
	if result == 0 {
		return 0
	}
	return result
}

func statsigCubicBezier(t, x1, y1, x2, y2 float64) float64 {
	bezier := func(u float64) (float64, float64) {
		remaining := 1 - u
		first := 3 * remaining * remaining * u
		second := 3 * remaining * u * u
		third := u * u * u
		return first*x1 + second*x2 + third, first*y1 + second*y2 + third
	}
	low, high := 0.0, 1.0
	for range 80 {
		middle := (low + high) / 2
		x, _ := bezier(middle)
		if x < t {
			low = middle
		} else {
			high = middle
		}
	}
	_, y := bezier((low + high) / 2)
	return y
}

func simulateStatsigStyle(values []int, c int) (string, string, error) {
	if len(values) < 11 {
		return "", "", fmt.Errorf("Statsig 动画参数不足")
	}
	currentTime := math.Floor(float64(c)/10+0.5) * 10
	t := currentTime / 4096
	controls := make([]float64, 4)
	for index, value := range values[7:11] {
		parameter := 0.0
		if index%2 == 1 {
			parameter = -1
		}
		controls[index] = statsigH(float64(value), parameter, 1, false)
	}
	eased := statsigCubicBezier(t, controls[0], controls[1], controls[2], controls[3])
	components := make([]int, 3)
	for index := range components {
		start, end := float64(values[index]), float64(values[index+3])
		components[index] = int(math.Floor(start + (end-start)*eased + 0.5))
	}
	color := fmt.Sprintf("rgb(%d, %d, %d)", components[0], components[1], components[2])
	endAngle := statsigH(float64(values[6]), 60, 360, true)
	radians := endAngle * eased * math.Pi / 180
	cosine, sine := math.Cos(radians), math.Sin(radians)
	format := func(value float64, precision int) string {
		if math.Abs(value) < 1e-7 {
			return "0"
		}
		if math.Abs(value-math.Round(value)) < 1e-7 {
			return strconv.FormatInt(int64(math.Round(value)), 10)
		}
		return strconv.FormatFloat(value, 'f', precision, 64)
	}
	transform := fmt.Sprintf("matrix(%s, %s, %s, %s, 0, 0)", format(cosine, 6), format(sine, 7), format(-sine, 7), format(cosine, 6))
	return color, transform, nil
}

func statsigNumberHex(value float64) string {
	rounded := math.Round(value*100) / 100
	if rounded == 0 {
		return "0"
	}
	sign := ""
	if math.Signbit(rounded) {
		sign = "-"
	}
	absolute := math.Abs(rounded)
	integer := int64(math.Floor(absolute))
	fraction := absolute - float64(integer)
	if fraction == 0 {
		return sign + strconv.FormatInt(integer, 16)
	}
	digits := strings.Builder{}
	for range 20 {
		fraction *= 16
		digit := int(math.Floor(fraction + 1e-12))
		digits.WriteString(strconv.FormatInt(int64(digit), 16))
		fraction -= float64(digit)
		if math.Abs(fraction) < 1e-12 {
			break
		}
	}
	fractionHex := strings.TrimRight(digits.String(), "0")
	if fractionHex == "" {
		return sign + strconv.FormatInt(integer, 16)
	}
	return sign + strconv.FormatInt(integer, 16) + "." + fractionHex
}
