package usecase

import "testing"

func TestResolveDocumentMIME(t *testing.T) {
	tests := []struct {
		name     string
		filename string
		wantMIME string
	}{
		{
			name:     "Docx",
			filename: "document.docx",
			wantMIME: "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
		},
		{
			name:     "Xlsx",
			filename: "spreadsheet.xlsx",
			wantMIME: "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
		},
		{
			name:     "Pptx",
			filename: "presentation.pptx",
			wantMIME: "application/vnd.openxmlformats-officedocument.presentationml.presentation",
		},
		{
			name:     "Zip",
			filename: "archive.zip",
			wantMIME: "application/zip",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := resolveDocumentMIME(tt.filename, []byte("dummy"))
			if got != tt.wantMIME {
				t.Fatalf("resolveDocumentMIME() = %q, want %q", got, tt.wantMIME)
			}
		})
	}
}

func TestIsLikelyOpusOgg(t *testing.T) {
	tests := []struct {
		name string
		data []byte
		want bool
	}{
		{
			name: "ValidOggOpusMarkers",
			data: append([]byte("0000OggSxxxxOpusHeadyyyy"), make([]byte, 80)...),
			want: true,
		},
		{
			name: "OggWithoutOpus",
			data: append([]byte("0000OggSxxxxVorbisHeaderyyyy"), make([]byte, 80)...),
			want: false,
		},
		{
			name: "NonOggData",
			data: []byte("ID3\x04\x00\x00some-mp3-data"),
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isLikelyOpusOgg(tt.data)
			if got != tt.want {
				t.Fatalf("isLikelyOpusOgg() = %v, want %v", got, tt.want)
			}
		})
	}
}
