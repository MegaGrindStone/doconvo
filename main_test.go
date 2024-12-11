package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/philippgille/chromem-go"
	bolt "go.etcd.io/bbolt"
)

func setupTestDB(t *testing.T) (*bolt.DB, string) {
	tempDir, err := os.MkdirTemp("", "doconvo-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}

	dbPath := filepath.Join(tempDir, "test.db")
	db, err := bolt.Open(dbPath, 0600, nil)
	if err != nil {
		t.Fatalf("Failed to open test database: %v", err)
	}

	err = initKVDB(db)
	if err != nil {
		t.Fatalf("Failed to initialize test database: %v", err)
	}

	return db, tempDir
}

func setupTestVectorDB(t *testing.T, tempDir string) *chromem.DB {
	vectordbPath := filepath.Join(tempDir, "vectordb")
	vectordb, err := chromem.NewPersistentDB(vectordbPath, false)
	if err != nil {
		t.Fatalf("Failed to create vector database: %v", err)
	}
	return vectordb
}

func TestNewMainModel(t *testing.T) {
	db, tempDir := setupTestDB(t)
	defer os.RemoveAll(tempDir)
	defer db.Close()

	vectordb := setupTestVectorDB(t, tempDir)

	model, err := newMainModel(db, vectordb)
	if err != nil {
		t.Errorf("newMainModel() error = %v, want nil", err)
	}
	// Empty config would redirect user to viewStateOptions
	if model.viewState != viewStateOptions {
		t.Errorf("newMainModel() viewState = %v, want %v", model.viewState, viewStateOptions)
	}
}

func TestSetViewState(t *testing.T) {
	db, tempDir := setupTestDB(t)
	defer os.RemoveAll(tempDir)
	defer db.Close()

	vectordb := setupTestVectorDB(t, tempDir)

	model, err := newMainModel(db, vectordb)
	if err != nil {
		t.Fatalf("Failed to create model: %v", err)
	}

	tests := []struct {
		name          string
		newViewState  viewState
		expectedState viewState
	}{
		{"Set to Chat", viewStateChat, viewStateChat},
		{"Set to Options", viewStateOptions, viewStateOptions},
		{"Set to Documents", viewStateDocuments, viewStateDocuments},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			updatedModel := model.setViewState(tt.newViewState)
			if updatedModel.viewState != tt.expectedState {
				t.Errorf("setViewState() viewState = %v, want %v", updatedModel.viewState, tt.expectedState)
			}
			if updatedModel.keymap.viewState != tt.expectedState {
				t.Errorf("setViewState() keymap.viewState = %v, want %v", updatedModel.keymap.viewState, tt.expectedState)
			}
		})
	}
}

func TestUpdateFormSize(t *testing.T) {
	db, tempDir := setupTestDB(t)
	defer os.RemoveAll(tempDir)
	defer db.Close()

	vectordb := setupTestVectorDB(t, tempDir)

	model, err := newMainModel(db, vectordb)
	if err != nil {
		t.Fatalf("Failed to create model: %v", err)
	}

	testCases := []struct {
		width  int
		height int
	}{
		{80, 24},
		{100, 30},
		{120, 40},
	}

	for _, tc := range testCases {
		t.Run("WindowSize", func(t *testing.T) {
			model.width = tc.width
			model.height = tc.height

			updatedModel := model.updateFormSize()

			if updatedModel.formWidth != tc.width {
				t.Errorf("updateFormSize() formWidth = %v, want %v", updatedModel.formWidth, tc.width)
			}
			if updatedModel.formHeight <= 0 || updatedModel.formHeight >= tc.height {
				t.Errorf("updateFormSize() formHeight = %v, should be between 0 and %v", updatedModel.formHeight, tc.height)
			}
		})
	}
}

func TestInitLogger(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "doconvo-log-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}
	defer os.RemoveAll(tempDir)

	tests := []struct {
		name    string
		debug   bool
		wantErr bool
	}{
		{"Debug enabled", true, false},
		{"Debug disabled", false, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := initLogger(tempDir, tt.debug)
			if (err != nil) != tt.wantErr {
				t.Errorf("initLogger() error = %v, wantErr %v", err, tt.wantErr)
			}

			// Verify log file was created
			logPath := filepath.Join(tempDir, "doconvo.log")
			if _, err := os.Stat(logPath); err != nil {
				t.Errorf("Log file was not created at %s: %v", logPath, err)
			}
		})
	}
}
