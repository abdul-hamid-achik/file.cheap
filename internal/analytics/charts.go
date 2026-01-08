package analytics

import (
	"bytes"
	"fmt"
	"image/color"
	"time"

	"github.com/wcharczuk/go-chart/v2"
	"github.com/wcharczuk/go-chart/v2/drawing"
)

var (
	colorPrimary   = drawing.ColorFromHex("81A1C1")
	colorSecondary = drawing.ColorFromHex("A3BE8C")
	colorBg        = drawing.ColorFromHex("2E3440")
	colorGrid      = drawing.ColorFromHex("3B4252")
	colorText      = drawing.ColorFromHex("D8DEE9")
)

func (s *Service) GenerateUsageChart(data []DailyUsage, width, height int) ([]byte, error) {
	if len(data) == 0 {
		return generateEmptyChart(width, height, "No usage data")
	}

	var xValues []time.Time
	var uploadsY []float64
	var transformsY []float64

	for _, d := range data {
		xValues = append(xValues, d.Date)
		uploadsY = append(uploadsY, float64(d.Uploads))
		transformsY = append(transformsY, float64(d.Transforms))
	}

	graph := chart.Chart{
		Width:  width,
		Height: height,
		Background: chart.Style{
			FillColor: colorBg,
			Padding: chart.Box{
				Top:    20,
				Left:   20,
				Right:  20,
				Bottom: 20,
			},
		},
		Canvas: chart.Style{
			FillColor: colorBg,
		},
		XAxis: chart.XAxis{
			Style: chart.Style{
				StrokeColor: colorGrid,
				FontColor:   colorText,
				FontSize:    10,
			},
			ValueFormatter: chart.TimeDateValueFormatter,
			GridMajorStyle: chart.Style{
				StrokeColor: colorGrid,
				StrokeWidth: 1,
			},
		},
		YAxis: chart.YAxis{
			Style: chart.Style{
				StrokeColor: colorGrid,
				FontColor:   colorText,
				FontSize:    10,
			},
			GridMajorStyle: chart.Style{
				StrokeColor: colorGrid,
				StrokeWidth: 1,
			},
		},
		Series: []chart.Series{
			chart.TimeSeries{
				Name:    "Uploads",
				XValues: xValues,
				YValues: uploadsY,
				Style: chart.Style{
					StrokeColor: colorPrimary,
					StrokeWidth: 2,
					FillColor:   colorPrimary.WithAlpha(50),
				},
			},
			chart.TimeSeries{
				Name:    "Transforms",
				XValues: xValues,
				YValues: transformsY,
				Style: chart.Style{
					StrokeColor: colorSecondary,
					StrokeWidth: 2,
				},
			},
		},
	}

	graph.Elements = []chart.Renderable{
		chart.LegendThin(&graph, chart.Style{
			FillColor: colorBg,
			FontColor: colorText,
			FontSize:  10,
		}),
	}

	var buf bytes.Buffer
	if err := graph.Render(chart.PNG, &buf); err != nil {
		return nil, fmt.Errorf("render chart: %w", err)
	}
	return buf.Bytes(), nil
}

func (s *Service) GeneratePieChart(data []TransformStat, width, height int) ([]byte, error) {
	if len(data) == 0 {
		return generateEmptyChart(width, height, "No transform data")
	}

	colors := []drawing.Color{
		drawing.ColorFromHex("81A1C1"), // nord9 blue
		drawing.ColorFromHex("A3BE8C"), // nord14 green
		drawing.ColorFromHex("EBCB8B"), // nord13 yellow
		drawing.ColorFromHex("BF616A"), // nord11 red
		drawing.ColorFromHex("B48EAD"), // nord15 purple
		drawing.ColorFromHex("88C0D0"), // nord8 cyan
		drawing.ColorFromHex("5E81AC"), // nord10 deep blue
	}

	var values []chart.Value
	for i, d := range data {
		values = append(values, chart.Value{
			Label: fmt.Sprintf("%s (%.0f%%)", d.Type, d.Percentage),
			Value: float64(d.Count),
			Style: chart.Style{
				FillColor: colors[i%len(colors)],
				FontColor: colorText,
				FontSize:  10,
			},
		})
	}

	pie := chart.DonutChart{
		Width:  width,
		Height: height,
		Values: values,
		Background: chart.Style{
			FillColor: colorBg,
		},
	}

	var buf bytes.Buffer
	if err := pie.Render(chart.PNG, &buf); err != nil {
		return nil, fmt.Errorf("render pie chart: %w", err)
	}
	return buf.Bytes(), nil
}

func (s *Service) GenerateRevenueChart(data []RevenuePoint, width, height int) ([]byte, error) {
	if len(data) == 0 {
		return generateEmptyChart(width, height, "No revenue data")
	}

	var xValues []time.Time
	var mrrValues []float64
	var userValues []float64

	for _, d := range data {
		xValues = append(xValues, d.Date)
		mrrValues = append(mrrValues, d.MRR)
		userValues = append(userValues, float64(d.Users))
	}

	graph := chart.Chart{
		Width:  width,
		Height: height,
		Background: chart.Style{
			FillColor: colorBg,
			Padding: chart.Box{
				Top:    20,
				Left:   50,
				Right:  50,
				Bottom: 20,
			},
		},
		Canvas: chart.Style{
			FillColor: colorBg,
		},
		XAxis: chart.XAxis{
			Style: chart.Style{
				StrokeColor: colorGrid,
				FontColor:   colorText,
				FontSize:    10,
			},
			ValueFormatter: chart.TimeDateValueFormatter,
		},
		YAxis: chart.YAxis{
			Name: "MRR ($)",
			NameStyle: chart.Style{
				FontColor: colorText,
				FontSize:  10,
			},
			Style: chart.Style{
				StrokeColor: colorGrid,
				FontColor:   colorText,
				FontSize:    10,
			},
			ValueFormatter: func(v interface{}) string {
				return fmt.Sprintf("$%.0f", v.(float64))
			},
		},
		YAxisSecondary: chart.YAxis{
			Name: "Users",
			NameStyle: chart.Style{
				FontColor: colorText,
				FontSize:  10,
			},
			Style: chart.Style{
				StrokeColor: colorGrid,
				FontColor:   colorText,
				FontSize:    10,
			},
		},
		Series: []chart.Series{
			chart.TimeSeries{
				Name:    "MRR",
				XValues: xValues,
				YValues: mrrValues,
				Style: chart.Style{
					StrokeColor: colorPrimary,
					StrokeWidth: 2.5,
					FillColor:   colorPrimary.WithAlpha(40),
				},
			},
			chart.TimeSeries{
				Name:    "Users",
				YAxis:   chart.YAxisSecondary,
				XValues: xValues,
				YValues: userValues,
				Style: chart.Style{
					StrokeColor:     colorSecondary,
					StrokeWidth:     2,
					StrokeDashArray: []float64{5, 3},
				},
			},
		},
	}

	graph.Elements = []chart.Renderable{
		chart.LegendThin(&graph, chart.Style{
			FillColor: colorBg,
			FontColor: colorText,
			FontSize:  10,
		}),
	}

	var buf bytes.Buffer
	if err := graph.Render(chart.PNG, &buf); err != nil {
		return nil, fmt.Errorf("render revenue chart: %w", err)
	}
	return buf.Bytes(), nil
}

func generateEmptyChart(width, height int, message string) ([]byte, error) {
	graph := chart.Chart{
		Width:  width,
		Height: height,
		Background: chart.Style{
			FillColor: colorBg,
		},
		Canvas: chart.Style{
			FillColor: colorBg,
		},
		Series: []chart.Series{
			chart.AnnotationSeries{
				Annotations: []chart.Value2{
					{XValue: float64(width / 2), YValue: float64(height / 2), Label: message},
				},
				Style: chart.Style{
					FontColor: colorText,
					FontSize:  14,
				},
			},
		},
	}

	var buf bytes.Buffer
	if err := graph.Render(chart.PNG, &buf); err != nil {
		return generatePlaceholder(width, height), nil
	}
	return buf.Bytes(), nil
}

func generatePlaceholder(width, height int) []byte {
	return nil
}

var _ color.Color = drawing.Color{}
