package service

import (
	"regexp"
	"strconv"
	"strings"
)

var plazaVersionNumberPattern = regexp.MustCompile(`\d+`)

// plazaVersionParts 从模型 id 抽出数字段（claude-fable-5-1 → [5, 1]，gpt-5.6 → [5, 6]）。
func plazaVersionParts(name string) []int {
	matches := plazaVersionNumberPattern.FindAllString(name, -1)
	parts := make([]int, 0, len(matches))
	for _, match := range matches {
		value, err := strconv.Atoi(match)
		if err != nil {
			continue
		}
		parts = append(parts, value)
	}
	return parts
}

// comparePlazaModelNames 版本号从新到旧；缺段当 0（带日期后缀比无日期新）；同版本按名称升序。
func comparePlazaModelNames(left, right string) int {
	leftParts := plazaVersionParts(left)
	rightParts := plazaVersionParts(right)
	n := len(leftParts)
	if len(rightParts) > n {
		n = len(rightParts)
	}
	for i := 0; i < n; i++ {
		leftVal := 0
		rightVal := 0
		if i < len(leftParts) {
			leftVal = leftParts[i]
		}
		if i < len(rightParts) {
			rightVal = rightParts[i]
		}
		if leftVal != rightVal {
			if leftVal > rightVal {
				return -1
			}
			return 1
		}
	}
	return strings.Compare(left, right)
}

func lessPlazaModels(left, right PlazaModel) bool {
	if cmp := comparePlazaModelNames(left.Name, right.Name); cmp != 0 {
		return cmp < 0
	}
	return left.Platform < right.Platform
}
