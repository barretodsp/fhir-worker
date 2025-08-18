package main

import (
	"context"
	"log"
	"os"
	"path/filepath"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sqs/types"
	"github.com/stretchr/testify/assert"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// 1. Definimos um tipo wrapper para o cliente MongoDB
type MongoDBWrapper struct {
	client *mongo.Client
}

func (w *MongoDBWrapper) Database(name string) *mongo.Database {
	return w.client.Database(name)
}

// 2. Teste para getDatabaseName
func TestGetDatabaseName(t *testing.T) {
	tests := []struct {
		name           string
		messageGroupId string
		want           string
	}{
		{"HC A", "001", "fhir_hca"},
		{"HC B", "002", "fhir_hcb"},
		{"Invalid", "999", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := getDatabaseName(tt.messageGroupId)
			assert.Equal(t, tt.want, got)
		})
	}
}

// 3. Teste para saveResource
func TestSaveResource(t *testing.T) {
	ctx := context.Background()

	// Mock da mensagem SQS
	mockMsg := &types.Message{
		MessageId: aws.String("test-message"),
		Body: aws.String(`{
			"encounter": {
				"fhirId": "123",
				"fullUrl": "urn:uuid:123",
				"status": "finished",
				"class": "outpatient",
				"period": {
					"start": "2023-01-01T10:00:00Z",
					"end": "2023-01-01T11:00:00Z"
				},
				"practitionerId": "dr-smith",
				"patientId": "patient-123"
			},
			"practitioner": {
				"fhirId": "dr-smith",
				"givenName": "John",
				"familyName": "Smith"
			},
			"patient": {
				"fhirId": "patient-123",
				"givenName": "Maria",
				"familyName": "Silva",
				"birthDate": "1990-01-01",
				"gender": "female"
			}
		}`),
	}

	t.Run("successful save", func(t *testing.T) {
		// Criamos um client MongoDB de teste
		client, err := mongo.Connect(ctx, options.Client().ApplyURI("mongodb://localhost:27017"))
		assert.NoError(t, err)
		defer client.Disconnect(ctx)

		// Envolvemos o client no wrapper
		wrappedClient := &mongo.Client{}

		err = saveResource(ctx, wrappedClient, "fhir_hca", mockMsg)
		assert.NoError(t, err)
	})

	t.Run("invalid message body", func(t *testing.T) {
		invalidMsg := &types.Message{
			Body: aws.String(`invalid json`),
		}

		// Client vazio para este teste
		wrappedClient := &mongo.Client{}

		err := saveResource(ctx, wrappedClient, "fhir_hca", invalidMsg)
		assert.Error(t, err)
	})
}

// 4. Teste para initLogger
func TestInitLogger(t *testing.T) {
	// Cria um diretório temporário
	tempDir := t.TempDir()
	tempLogPath := filepath.Join(tempDir, "logs")

	// Substitui temporariamente a variável global
	logPath := tempLogPath
	originalLogPath := logPath

	defer func() {
		// Restaura o valor original
		logPath = originalLogPath
	}()

	// Testa a função
	initLogger()

	// Verifica se podemos escrever logs
	log.Println("Test log message")

	// Verifica se os arquivos foram criados
	_, err := os.Stat(tempLogPath)
	assert.NoError(t, err)
}
