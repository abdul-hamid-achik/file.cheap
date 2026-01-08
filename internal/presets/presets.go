package presets

type Preset struct {
	Width   int
	Height  int
	Quality int
	Crop    bool
}

var Thumbnail = Preset{Width: 300, Height: 300, Quality: 85, Crop: true}

var PDFThumbnail = Preset{Width: 300, Height: 300, Quality: 85, Crop: false}

var PDFResponsive = map[string]Preset{
	"pdf_sm": {Width: 640, Height: 0, Quality: 85, Crop: false},
	"pdf_md": {Width: 1024, Height: 0, Quality: 85, Crop: false},
	"pdf_lg": {Width: 1920, Height: 0, Quality: 85, Crop: false},
}

var Responsive = map[string]Preset{
	"sm": {Width: 640, Height: 0, Quality: 85, Crop: false},
	"md": {Width: 1024, Height: 0, Quality: 85, Crop: false},
	"lg": {Width: 1920, Height: 0, Quality: 85, Crop: false},
	"xl": {Width: 2560, Height: 0, Quality: 85, Crop: false},
}

var Social = map[string]Preset{
	"og":                 {Width: 1200, Height: 630, Quality: 90, Crop: true},
	"twitter":            {Width: 1200, Height: 675, Quality: 90, Crop: true},
	"instagram_square":   {Width: 1080, Height: 1080, Quality: 90, Crop: true},
	"instagram_portrait": {Width: 1080, Height: 1350, Quality: 90, Crop: true},
	"instagram_story":    {Width: 1080, Height: 1920, Quality: 90, Crop: true},
}

var All = map[string]Preset{
	"thumbnail":          Thumbnail,
	"sm":                 Responsive["sm"],
	"md":                 Responsive["md"],
	"lg":                 Responsive["lg"],
	"xl":                 Responsive["xl"],
	"og":                 Social["og"],
	"twitter":            Social["twitter"],
	"instagram_square":   Social["instagram_square"],
	"instagram_portrait": Social["instagram_portrait"],
	"instagram_story":    Social["instagram_story"],
	"pdf_thumbnail":      PDFThumbnail,
	"pdf_sm":             PDFResponsive["pdf_sm"],
	"pdf_md":             PDFResponsive["pdf_md"],
	"pdf_lg":             PDFResponsive["pdf_lg"],
}

func Get(name string) (Preset, bool) {
	p, ok := All[name]
	return p, ok
}

func IsSocialPreset(name string) bool {
	_, ok := Social[name]
	return ok
}

func IsResponsivePreset(name string) bool {
	_, ok := Responsive[name]
	return ok
}

var SocialPresetNames = []string{
	"og",
	"twitter",
	"instagram_square",
	"instagram_portrait",
	"instagram_story",
}

var ResponsivePresetNames = []string{
	"sm",
	"md",
	"lg",
	"xl",
}
