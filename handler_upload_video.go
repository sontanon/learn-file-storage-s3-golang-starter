package main

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"mime"
	"net/http"
	"os"
	"os/exec"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/bootdotdev/learn-file-storage-s3-golang-starter/internal/auth"
	"github.com/google/uuid"
)

const GIGABYTE int = 1 << 30
const SIXTEEN_TO_NINE float64 = 16.0 / 9.0
const NINE_TO_SIXTEEN float64 = 9.0 / 16.0
const EPSILON float64 = 0.01

type aspectRatio string

const (
	aspectRatioInvalid       aspectRatio = ""
	aspectRatioSixteenToNine aspectRatio = "16:9"
	aspectRatioNineToSixteen aspectRatio = "9:16"
	aspectRatioOther         aspectRatio = "other"
)

func absCompare(a, b float64) bool {
	if a > b {
		return a-b < EPSILON
	}
	return b-a < EPSILON
}

func getVideoAspectRatio(filePath string) (aspectRatio, error) {
	cmd := exec.Command("ffprobe", "-v", "error", "-print_format", "json", "-show_streams", filePath)

	var buffer bytes.Buffer
	cmd.Stdout = &buffer

	if err := cmd.Run(); err != nil {
		return "", err
	}

	var result struct {
		Streams []struct {
			Width  int `json:"width"`
			Height int `json:"height"`
		} `json:"streams"`
	}

	if err := json.Unmarshal(buffer.Bytes(), &result); err != nil {
		return "", err
	}

	prettyJSON, _ := json.MarshalIndent(result, "", "  ")
	log.Printf("result: %s", string(prettyJSON))

	if len(result.Streams) == 0 {
		return "", fmt.Errorf("no streams found in video file")
	}
	width := result.Streams[0].Width
	height := result.Streams[0].Height
	log.Printf("Width: %d, Height: %d", width, height)

	if width == 0 || height == 0 {
		return "", fmt.Errorf("invalid video dimensions: %dx%d", width, height)
	}

	ratio := float64(width) / float64(height)
	if absCompare(ratio, SIXTEEN_TO_NINE) {
		return aspectRatioSixteenToNine, nil
	} else if absCompare(ratio, NINE_TO_SIXTEEN) {
		return aspectRatioNineToSixteen, nil
	} else {
		return aspectRatioOther, nil
	}
}

func (cfg *apiConfig) handlerUploadVideo(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, int64(GIGABYTE))

	videoIDString := r.PathValue("videoID")
	videoID, err := uuid.Parse(videoIDString)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid ID", err)
		return
	}

	token, err := auth.GetBearerToken(r.Header)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "Couldn't find JWT", err)
		return
	}

	userID, err := auth.ValidateJWT(token, cfg.jwtSecret)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "Couldn't validate JWT", err)
		return
	}

	video, err := cfg.db.GetVideo(videoID)
	if err != nil || video.UserID != userID {
		respondWithError(w, http.StatusUnauthorized, "Couldn't get video metadata", err)
		return
	}

	file, fileHeader, err := r.FormFile("video")
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Couldn't get 'video' form file", err)
		return
	}
	defer file.Close()

	mediaType, _, err := mime.ParseMediaType(fileHeader.Header.Get("Content-Type"))
	if err != nil || mediaType != "video/mp4" {
		respondWithError(w, http.StatusBadRequest, "Unable to parse media type", err)
		return
	}

	tempFile, err := os.CreateTemp("", "tubely-upload.mp4")
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Unable to create temporary file", err)
		return
	}
	defer os.Remove(tempFile.Name())
	defer tempFile.Close()

	_, err = io.Copy(tempFile, file)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Unable to copy to temporary file", err)
		return
	}

	_, err = tempFile.Seek(0, io.SeekStart)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Unable to reset temp file pointer", err)
		return
	}

	aspectRatio, err := getVideoAspectRatio(tempFile.Name())
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Unable to get video aspect ratio", err)
		return
	}

	var prefix string
	switch aspectRatio {
	case aspectRatioSixteenToNine:
		prefix = "landscape"
	case aspectRatioNineToSixteen:
		prefix = "portrait"
	case aspectRatioOther:
		prefix = "other"
	default:
		respondWithError(w, http.StatusInternalServerError, "Invalid aspect ratio", fmt.Errorf("invalid aspect ratio: %s", aspectRatio))
		return
	}

	randomBytes := make([]byte, 32)
	_, _ = rand.Read(randomBytes)
	key := fmt.Sprintf("%s/%s.mp4", prefix, base64.RawURLEncoding.EncodeToString(randomBytes))

	params := s3.PutObjectInput{
		Bucket:      &cfg.s3Bucket,
		Key:         &key,
		Body:        tempFile,
		ContentType: &mediaType,
	}
	_, err = cfg.s3Client.PutObject(r.Context(), &params)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Could not put object in S3", err)
		return
	}

	videoURL := fmt.Sprintf("https://%s.s3.%s.amazonaws.com/%s", cfg.s3Bucket, cfg.s3Region, key)

	video.VideoURL = &videoURL

	if err := cfg.db.UpdateVideo(video); err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't save updated video data", err)
		return
	}

	respondWithJSON(w, http.StatusOK, video)
}
