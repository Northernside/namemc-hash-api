package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"image"
	"image/draw"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"sync"

	"github.com/disintegration/imaging"
)

type HashResponse struct {
	Standard               string `json:"standard_hash"`
	AlphaNormalized        string `json:"alpha_normalized_hash"`
	AlphaNormalizedCompact string `json:"alpha_normalized_compact"`
}

var cache sync.Map

func main() {
	loadEnvironment()
	http.HandleFunc("/hash", recoverMiddleware(handleHash))
	log.Printf("Server running on http://%s:%s", getEnv("HOST"), getEnv("PORT"))
	log.Fatal(http.ListenAndServe(fmt.Sprintf("%s:%s", getEnv("HOST"), getEnv("PORT")), nil))
}

func recoverMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				http.Error(w, `{"error": "Internal server error", "details": "`+fmt.Sprintf("%v", rec)+`"}`, http.StatusInternalServerError)
			}
		}()
		next(w, r)
	}
}

func handleHash(w http.ResponseWriter, r *http.Request) {
	var skinBytes []byte
	var err error
	var cacheKey string

	url := r.URL.Query().Get("url")
	if url != "" {
		cleanedURL, err := normalizeURL(url)
		if err != nil {
			http.Error(w, `{"error": "Invalid URL", "details": "`+err.Error()+`"}`, http.StatusBadRequest)
			return
		}

		cacheKey = fmt.Sprintf("url:%s", cleanedURL)
		if val, ok := cache.Load(cacheKey); ok {
			writeJSON(w, val)
			return
		}

		resp, err := http.Get(url)
		if err != nil || resp.StatusCode != 200 {
			http.Error(w, `{"error": "Failed to fetch image from URL", "details": "`+err.Error()+`"}`, http.StatusBadRequest)
			return
		}
		defer resp.Body.Close()

		skinBytes, err = io.ReadAll(resp.Body)
		if err != nil {
			http.Error(w, `{"error": "Failed to read image from URL", "details": "`+err.Error()+`"}`, http.StatusInternalServerError)
			return
		}
	} else {
		file, _, err := r.FormFile("file")
		if err != nil {
			http.Error(w, `{"error": "Failed to get uploaded file", "details": "`+err.Error()+`"}`, http.StatusBadRequest)
			return
		}
		defer file.Close()

		skinBytes, err = io.ReadAll(file)
		if err != nil {
			http.Error(w, `{"error": "Failed to read uploaded file", "details": "`+err.Error()+`"}`, http.StatusInternalServerError)
			return
		}

		cacheKey = fmt.Sprintf("sha256:%s", sha256Hex(skinBytes))
		if val, ok := cache.Load(cacheKey); ok {
			writeJSON(w, val)
			return
		}
	}

	hashes, err := computeHashes(skinBytes)
	if err != nil {
		http.Error(w, `{"error": "Failed to compute hashes", "details": "`+err.Error()+`"}`, http.StatusInternalServerError)
		return
	}

	cache.Store(cacheKey, hashes)
	writeJSON(w, hashes)
}

func computeHashes(imgBytes []byte) (HashResponse, error) {
	img, format, err := image.Decode(bytes.NewReader(imgBytes))
	if err != nil {
		return HashResponse{}, fmt.Errorf("image decode failed: %v", err)
	}

	if strings.ToLower(format) != "png" {
		return HashResponse{}, fmt.Errorf("only PNG images are supported")
	}

	bounds := img.Bounds()
	width, height := bounds.Dx(), bounds.Dy()

	rgba := image.NewNRGBA(bounds)
	draw.Draw(rgba, bounds, img, bounds.Min, draw.Src)

	for y := range height {
		for x := range width {
			i := rgba.PixOffset(x, y)
			if rgba.Pix[i+3] == 0 {
				rgba.Pix[i+0] = 0
				rgba.Pix[i+1] = 0
				rgba.Pix[i+2] = 0
			}
		}
	}

	header := make([]byte, 8)
	binary.BigEndian.PutUint32(header[0:], uint32(width))
	binary.BigEndian.PutUint32(header[4:], uint32(height))

	alphaBuffer := append(header, rgba.Pix...)
	alphaHash := hashBuffer(alphaBuffer)

	buffer := new(bytes.Buffer)
	err = imaging.Encode(buffer, img, imaging.PNG)
	if err != nil {
		return HashResponse{}, err
	}

	standardHash := hashBuffer(buffer.Bytes())
	return HashResponse{
		Standard:               standardHash,
		AlphaNormalized:        alphaHash,
		AlphaNormalizedCompact: alphaHash[:16],
	}, nil
}

func hashBuffer(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func writeJSON(w http.ResponseWriter, data any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}

func normalizeURL(raw string) (string, error) {
	parsed, err := url.Parse(raw)
	if err != nil {
		return raw, err
	}

	parsed.RawQuery = ""
	parsed.Fragment = ""
	return parsed.String(), nil
}
