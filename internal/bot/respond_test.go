package bot

import (
	"strconv"
	"testing"
	"time"

	"github.com/bwmarrin/discordgo"

	"github.com/cyblster/soundmate/internal/ui"
)

func snowflakeAt(t time.Time) string {
	return strconv.FormatInt((t.UnixMilli()-1420070400000)<<22, 10)
}

func TestInteractionName(t *testing.T) {
	tests := []struct {
		e    *discordgo.InteractionCreate
		want string
	}{
		{command(CommandAdd), "/add"},
		{button(ui.IDSkip), ui.IDSkip},
		{button(ui.SelectID("abc")), ui.SelectID("")},
		{interaction(discordgo.InteractionModalSubmit, discordgo.ModalSubmitInteractionData{CustomID: ui.IDAddModal}, nil), ui.IDAddModal},
	}
	for _, tt := range tests {
		if got := interactionName(tt.e.Interaction); got != tt.want {
			t.Errorf("interactionName = %q, want %q", got, tt.want)
		}
	}
}

func TestInteractionAge(t *testing.T) {
	i := &discordgo.Interaction{ID: snowflakeAt(time.Now().Add(-5 * time.Second))}
	if age := interactionAge(i); age < 4*time.Second || age > 10*time.Second {
		t.Errorf("unexpected age %v", age)
	}
	if age := interactionAge(&discordgo.Interaction{ID: "bad"}); age != 0 {
		t.Errorf("invalid id must give zero age, got %v", age)
	}
}
