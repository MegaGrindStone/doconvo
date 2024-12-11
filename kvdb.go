package main

import (
	"encoding/binary"
	"encoding/json"

	bolt "go.etcd.io/bbolt"
)

const (
	sessionsBucket            = "sessions"
	documentsBucket           = "documents"
	llmProviderSettingsBucket = "llmProviderSettings"
	llmSettingsBucket         = "llmSettings"
)

func initKVDB(db *bolt.DB) error {
	return db.Update(func(tx *bolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists([]byte(sessionsBucket))
		if err != nil {
			return err
		}
		_, err = tx.CreateBucketIfNotExists([]byte(documentsBucket))
		if err != nil {
			return err
		}
		_, err = tx.CreateBucketIfNotExists([]byte(llmProviderSettingsBucket))
		if err != nil {
			return err
		}
		_, err = tx.CreateBucketIfNotExists([]byte(llmSettingsBucket))
		if err != nil {
			return err
		}

		return nil
	})
}

func loadSessions(db *bolt.DB) ([]session, error) {
	var sessions []session

	err := db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(sessionsBucket))

		return b.ForEach(func(k, v []byte) error {
			sess, err := decodeSession(v)
			if err != nil {
				return err
			}
			sessions = append(sessions, *sess)
			return nil
		})
	})

	return sessions, err
}

func saveSession(db *bolt.DB, sess *session) error {
	return db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(sessionsBucket))

		if sess.ID == 0 {
			id, _ := b.NextSequence()
			sess.ID = int(id)
		}

		data, err := json.Marshal(sess)
		if err != nil {
			return err
		}

		return b.Put(itob(sess.ID), data)
	})
}

func deleteSession(db *bolt.DB, id int) error {
	return db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(sessionsBucket))
		return b.Delete(itob(id))
	})
}

func loadDocuments(db *bolt.DB) ([]document, error) {
	var documents []document

	err := db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(documentsBucket))

		return b.ForEach(func(k, v []byte) error {
			doc, err := decodeDocument(v)
			if err != nil {
				return err
			}
			documents = append(documents, *doc)
			return nil
		})
	})

	return documents, err
}

func saveDocument(db *bolt.DB, doc *document) error {
	return db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(documentsBucket))

		if doc.ID == 0 {
			id, _ := b.NextSequence()
			doc.ID = int(id)
		}

		data, err := json.Marshal(doc)
		if err != nil {
			return err
		}

		return b.Put(itob(doc.ID), data)
	})
}

func deleteDocument(db *bolt.DB, id int) error {
	return db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(documentsBucket))
		return b.Delete(itob(id))
	})
}

func loadOllamaSettings(db *bolt.DB) (ollamaProvider, error) {
	var ollama ollamaProvider

	err := db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(llmProviderSettingsBucket))

		data := b.Get([]byte("ollama"))
		if data == nil {
			return nil
		}

		err := json.Unmarshal(data, &ollama)
		if err != nil {
			return err
		}

		return nil
	})

	return ollama, err
}

func saveOllamaSettings(db *bolt.DB, ollama ollamaProvider) error {
	return db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(llmProviderSettingsBucket))

		data, err := json.Marshal(ollama)
		if err != nil {
			return err
		}

		return b.Put([]byte("ollama"), data)
	})
}

func loadAnthropicSettings(db *bolt.DB) (anthropicProvider, error) {
	var anthro anthropicProvider

	err := db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(llmProviderSettingsBucket))

		data := b.Get([]byte("anthropic"))
		if data == nil {
			return nil
		}

		err := json.Unmarshal(data, &anthro)
		if err != nil {
			return err
		}

		return nil
	})

	return anthro, err
}

func saveAnthropicSettings(db *bolt.DB, anthro anthropicProvider) error {
	return db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(llmProviderSettingsBucket))

		data, err := json.Marshal(anthro)
		if err != nil {
			return err
		}

		return b.Put([]byte("anthropic"), data)
	})
}

func loadOpenAISettings(db *bolt.DB) (openaiProvider, error) {
	var openai openaiProvider

	err := db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(llmProviderSettingsBucket))

		data := b.Get([]byte("openai"))
		if data == nil {
			return nil
		}

		err := json.Unmarshal(data, &openai)
		if err != nil {
			return err
		}

		return nil
	})

	return openai, err
}

func saveOpenAISettings(db *bolt.DB, openai openaiProvider) error {
	return db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(llmProviderSettingsBucket))

		data, err := json.Marshal(openai)
		if err != nil {
			return err
		}

		return b.Put([]byte("openai"), data)
	})
}

func loadLLMSettings(db *bolt.DB, roles string) (llmSetting, error) {
	var llm llmSetting

	err := db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(llmSettingsBucket))

		data := b.Get([]byte(roles))
		if data == nil {
			return nil
		}

		err := json.Unmarshal(data, &llm)
		if err != nil {
			return err
		}

		return nil
	})

	return llm, err
}

func saveLLMSettings(db *bolt.DB, roles string, llm llmSetting) error {
	return db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(llmSettingsBucket))

		data, err := json.Marshal(llm)
		if err != nil {
			return err
		}

		return b.Put([]byte(roles), data)
	})
}

func decodeSession(data []byte) (*session, error) {
	var s session
	err := json.Unmarshal(data, &s)
	return &s, err
}

func decodeDocument(data []byte) (*document, error) {
	var d document
	err := json.Unmarshal(data, &d)
	return &d, err
}

func itob(v int) []byte {
	b := make([]byte, 8)
	binary.BigEndian.PutUint64(b, uint64(v))
	return b
}
