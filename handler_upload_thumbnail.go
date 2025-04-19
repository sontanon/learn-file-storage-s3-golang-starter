package main

import (
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"

	"github.com/bootdotdev/learn-file-storage-s3-golang-starter/internal/auth"
	"github.com/google/uuid"
)

func (cfg *apiConfig) handlerUploadThumbnail(w http.ResponseWriter, r *http.Request) {
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

	fmt.Println("uploading thumbnail for video", videoID, "by user", userID)

	const maxMemory = 10 << 20

	if err := r.ParseMultipartForm(maxMemory); err != nil {
		respondWithError(w, http.StatusInternalServerError, "Unable to parse multipart form", err)
		return
	}

	file, header, err := r.FormFile("thumbnail")
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Unable to parse form file", err)
		return
	}
	defer file.Close()

	mediaType := header.Header.Get("Content-Type")
	if mediaType == "" {
		respondWithError(w, http.StatusBadRequest, "No Content-Type in header", err)
		return
	}

	mediaType, _, err = mime.ParseMediaType(mediaType)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Unable to parse media type", err)
		return
	}

	var extension string
	switch mediaType {
	case "image/jpeg":
		extension = "jpg"
	case "image/png":
		extension = "png"
	default:
		respondWithError(w, http.StatusBadRequest, "Unsupported media type", fmt.Errorf("parsed media type %s is not 'image/jpeg' or 'image/png'", mediaType))
		return
	}

	fpath := filepath.Join(cfg.assetsRoot, fmt.Sprintf("%s.%s", videoIDString, extension))
	localFile, err := os.Create(fpath)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Unable to create file for thumbnail", err)
		return
	}

	if _, err := io.Copy(localFile, file); err != nil {
		respondWithError(w, http.StatusInternalServerError, "Error copying multipart file into local file", err)
		return
	}

	video, err := cfg.db.GetVideo(videoID)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "Couldn't get video metadata", err)
		return
	}

	dataURL := fmt.Sprintf("http://localhost:%s/assets/%s.%s", cfg.port, videoIDString, extension)
	video.ThumbnailURL = &dataURL

	if err := cfg.db.UpdateVideo(video); err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't save updated video data", err)
		return
	}

	respondWithJSON(w, http.StatusOK, video)
}
