package i18n

import (
	"reflect"
	"regexp"
	"testing"

	"github.com/bwmarrin/discordgo"
)

var verbPattern = regexp.MustCompile(`%[-+# 0]*[0-9]*[a-zA-Z]`)

func TestCatalogsAreComplete(t *testing.T) {
	for _, locale := range Supported() {
		value := reflect.ValueOf(*Get(locale))
		for i := range value.NumField() {
			if value.Field(i).String() == "" {
				t.Errorf("locale %q: message %s is empty", locale, value.Type().Field(i).Name)
			}
		}
	}
}

func TestCatalogsHaveSameFormatVerbs(t *testing.T) {
	reference := reflect.ValueOf(*Get(Default))
	for _, locale := range Supported() {
		value := reflect.ValueOf(*Get(locale))
		for i := range value.NumField() {
			want := verbPattern.FindAllString(reference.Field(i).String(), -1)
			got := verbPattern.FindAllString(value.Field(i).String(), -1)
			if !reflect.DeepEqual(want, got) {
				t.Errorf("locale %q: message %s has verbs %v, want %v", locale, value.Type().Field(i).Name, got, want)
			}
		}
	}
}

func TestParse(t *testing.T) {
	tests := []struct {
		input   string
		want    Locale
		wantErr bool
	}{
		{"en", English, false},
		{"RU", Russian, false},
		{" ru ", Russian, false},
		{"de", "", true},
		{"", "", true},
	}
	for _, tt := range tests {
		got, err := Parse(tt.input)
		if (err != nil) != tt.wantErr {
			t.Errorf("Parse(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
		}
		if got != tt.want {
			t.Errorf("Parse(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestGetFallsBackToDefault(t *testing.T) {
	if Get("xx") != Get(Default) {
		t.Fatal("unknown locale must fall back to the default catalog")
	}
	if Get(Russian).ActionAdd != "Добавить" {
		t.Fatal("russian catalog is not wired")
	}
}

func TestLocalizations(t *testing.T) {
	result := *Localizations(func(m *Messages) string { return m.ActionAdd })
	if result[discordgo.Russian] != "Добавить" || result[discordgo.EnglishUS] != "Add" || result[discordgo.EnglishGB] != "Add" {
		t.Fatalf("unexpected localizations: %v", result)
	}
	for discord, text := range result {
		l, ok := FromDiscord(discord)
		if !ok || Get(l).ActionAdd != text {
			t.Errorf("localization for %s must match the locale used for private messages", discord)
		}
	}
	if _, ok := FromDiscord(discordgo.German); ok {
		t.Error("unsupported discord locales must not be mapped")
	}
}
