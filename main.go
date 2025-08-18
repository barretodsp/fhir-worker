package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/aws/aws-sdk-go-v2/service/sqs/types"
	rotatelogs "github.com/lestrrat-go/file-rotatelogs"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type FHIRMessage struct {
	Encounter    Encounter    `json:"encounter"`
	Practitioner Practitioner `json:"practitioner"`
	Patient      Patient      `json:"patient"`
}

type Encounter struct {
	FhirId         string `json:"fhirId"`
	FullUrl        string `json:"fullUrl"`
	Status         string `json:"status"`
	Class          string `json:"class"`
	Period         Period `json:"period"`
	PractitionerId string `json:"practitionerId"`
	PatientId      string `json:"patientId"`
}

type Practitioner struct {
	FhirId     string `json:"fhirId"`
	GivenName  string `json:"givenName"`
	FamilyName string `json:"familyName"`
}

type Name struct {
	Family string   `json:"family"`
	Given  []string `json:"given"`
}

type Patient struct {
	FhirId     string `json:"id"`
	GivenName  string `json:"givenName"`
	FamilyName string `json:"familyName"`
	BirthDate  string `json:"birthDate"`
	Gender     string `json:"gender"`
}

type Class struct {
	System string `json:"system"`
	Code   string `json:"code"`
}

type Period struct {
	Start time.Time `json:"start"`
	End   time.Time `json:"end"`
}

type Participant struct {
	Individual Reference `json:"individual"`
}

type Reference struct {
	Reference string `json:"reference"`
}

func initLogger() {
	writer, err := rotatelogs.New(
		"/app/logs/worker.%Y-%m-%d.log",
		rotatelogs.WithLinkName("/app/logs/worker.log"),
		rotatelogs.WithRotationTime(24*time.Hour),
		rotatelogs.WithMaxAge(72*time.Hour),
	)
	if err != nil {
		log.Fatalf("Erro ao configurar rotação de logs: %v", err)
	}

	multi := io.MultiWriter(os.Stdout, writer)
	log.SetOutput(multi)
}

func getDatabaseName(messageGroupId string) string {
	switch messageGroupId {
	case "001":
		return "fhir_hca"
	case "002":
		return "fhir_hcb"
	default:
		return ""
	}
}

func getDbClient(ctx context.Context) (*mongo.Client, error) {
	dbURI := os.Getenv("DB_URI")
	dbUser := os.Getenv("DB_USER")
	dbPwd := os.Getenv("DB_PWD")

	if dbURI == "" || dbUser == "" || dbPwd == "" {
		log.Fatalf("DB credentials must be provided!")
	}

	client, err := mongo.Connect(ctx, options.Client().
		ApplyURI(dbURI).
		SetAuth(options.Credential{
			Username: dbUser,
			Password: dbPwd,
		}))

	if err != nil {
		log.Fatalf("Failed to connect to Database: %v", err)
	}

	err = client.Ping(ctx, nil)
	if err != nil {
		return nil, err
	}

	return client, nil
}

func saveResource(ctx context.Context, mongoClient *mongo.Client, dbName string, msg *types.Message) error {
	var fhirMsg FHIRMessage
	if err := json.Unmarshal([]byte(*msg.Body), &fhirMsg); err != nil {
		log.Printf("erro ao decodificar mensagem: %v", err)
		return err
	}

	log.Printf("Processando mensagem ID: %s", *msg.MessageId)

	log.Printf("Inserindo Patient ID: %s", fhirMsg.Patient.FhirId)
	patientCollection := mongoClient.Database(dbName).Collection("patients")
	patientDoc := bson.M{
		"fhirId":      fhirMsg.Encounter.FhirId,
		"givenName":   fhirMsg.Patient.GivenName,
		"familyName":  fhirMsg.Patient.FamilyName,
		"birthDate":   fhirMsg.Patient.BirthDate,
		"gender":      fhirMsg.Patient.Gender,
		"processedAt": time.Now(),
	}

	resultPatient, err := patientCollection.InsertOne(ctx, patientDoc)
	if err != nil {
		return fmt.Errorf("erro ao inserir patient: %v", err)
	}
	log.Printf("Patient %s registrado com sucesso", fhirMsg.Encounter.FhirId)

	log.Printf("Inserindo Practitioner ID: %s", fhirMsg.Practitioner.FhirId)
	pracCollection := mongoClient.Database(dbName).Collection("practitioners")
	pracDoc := bson.M{
		"fhirId":      fhirMsg.Encounter.FhirId,
		"givenName":   fhirMsg.Practitioner.GivenName,
		"familyName":  fhirMsg.Practitioner.FamilyName,
		"processedAt": time.Now(),
	}

	resultPrac, err := pracCollection.InsertOne(ctx, pracDoc)
	if err != nil {
		return fmt.Errorf("erro ao inserir Practitioner: %v", err)
	}
	log.Printf("Practitioner %s registrado com sucesso", fhirMsg.Practitioner.FhirId)

	log.Printf("Inserindo Encounter ID: %s", fhirMsg.Encounter.FhirId)

	encounterCollection := mongoClient.Database(dbName).Collection("encounters")
	encounterDoc := bson.M{
		"fhirId":         fhirMsg.Encounter.FhirId,
		"fullUrl":        fhirMsg.Encounter.FullUrl,
		"status":         fhirMsg.Encounter.Status,
		"class":          fhirMsg.Encounter.Class,
		"period":         fhirMsg.Encounter.Period,
		"practitionerId": resultPrac.InsertedID,
		"patientId":      resultPatient.InsertedID,
		"processedAt":    time.Now(),
	}

	if _, err := encounterCollection.InsertOne(ctx, encounterDoc); err != nil {
		return fmt.Errorf("erro ao inserir encounter: %v", err)
	}
	log.Printf("Encounter %s registrado com sucesso", fhirMsg.Encounter.FhirId)

	return nil
}

func getSqsClient(ctx context.Context) (*sqs.Client, error) {

	cfg, err := config.LoadDefaultConfig(ctx,
		config.WithRegion("sa-east-1"),
		config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider("test", "test", "")),
		config.WithEndpointResolverWithOptions(aws.EndpointResolverWithOptionsFunc(
			func(service, region string, options ...interface{}) (aws.Endpoint, error) {
				return aws.Endpoint{
					URL:           "http://localstack:4566",
					SigningRegion: "sa-east-1",
				}, nil
			},
		)),
	)

	if err != nil {
		log.Fatalf("Erro na configuração AWS: %v", err)
		return nil, err
	}

	return sqs.NewFromConfig(cfg), nil
}

func consumeMessage(ctx context.Context, sqsClient *sqs.Client, queueURL string) (*types.Message, error) {
	log.Printf("Consuming one message...")
	output, err := sqsClient.ReceiveMessage(ctx, &sqs.ReceiveMessageInput{
		QueueUrl:            aws.String(queueURL),
		MaxNumberOfMessages: 1,
		WaitTimeSeconds:     5,
		AttributeNames:      []types.QueueAttributeName{"MessageGroupId"},
	})

	if err != nil {
		return nil, err
	}

	if len(output.Messages) == 0 {
		log.Printf("No message available")
		return nil, nil
	}

	return &output.Messages[0], nil
}

func printFormattedJSON(raw string) {
	var obj interface{}

	if err := json.Unmarshal([]byte(raw), &obj); err != nil {
		fmt.Println(raw)
		return
	}

	pretty, err := json.MarshalIndent(obj, "", "  ")
	if err != nil {
		fmt.Println(raw)
		return
	}

	fmt.Println(string(pretty))
}

func processResources(ctx context.Context, sqsClient *sqs.Client, queueURL string) {
	for {
		log.Printf("Etapa 1 - Consumindo MSG")
		msg, err := consumeMessage(ctx, sqsClient, queueURL)
		if err != nil {
			log.Printf("Erro ao consumir mensagem: %v", err)
			time.Sleep(5 * time.Second)
			continue
		}

		if msg == nil {
			time.Sleep(1 * time.Second)
			continue
		}

		messageGroupID := msg.Attributes["MessageGroupId"]
		log.Printf("Etapa 2 - GroupId: %s", messageGroupID)
		fmt.Println("\n=== METADADOS DA MENSAGEM ===")
		fmt.Printf("ID: %s\n", *msg.MessageId)

		dbName := getDatabaseName(messageGroupID)
		if dbName == "" {
			log.Printf("Message with invalid messageGroupID: %s", messageGroupID)
			continue
		}

		log.Printf("Etapa 3 - Conectando-se ao Database")
		db, err := getDbClient(ctx)
		if err != nil {
			log.Printf("Erro ao conectar ao database: %v", err)
			continue
		}

		log.Printf("Etapa 4 - Registrando Resource ")
		if err := saveResource(ctx, db, dbName, msg); err != nil {
			log.Printf("Erro ao processar mensagem: %v", err)
			continue
		}

		log.Printf("FIM das Etapas")

	}
}

func main() {
	ctx := context.Background()
	initLogger()

	sqsClient, err := getSqsClient(ctx)
	if err != nil {
		log.Fatalf("Error while getting sqs client: %v", err)
	}

	sqsQueueURL := os.Getenv("SQS_QUEUE_URL")
	if sqsQueueURL == "" {
		log.Fatal("SQS_QUEUE_URL is empty.")
	}

	processResources(
		ctx,
		sqsClient,
		sqsQueueURL,
	)
}
