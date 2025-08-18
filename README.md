# fhir-worker


# Registro de Decisão de Arquitetura: Serviço Worker

**Status:** Proposto  
**Data:** 18/08/2025  
**Autor:** Patrick Barreto


**Versão:** 1.0


## Contexto:
Precisamos processar mensagens FHIR de uma fila SQS e armazená-las em bancos de dados MongoDB. O serviço deve:

1. Lidar com diferentes clientes de saúde (identificados por MessageGroupId)
2. Processar três recursos FHIR principais (Paciente, Profissional, Encontro)
3. Manter a integridade dos dados entre recursos relacionados
4. Ser resiliente e registrar todas as operações

## Decisão:
Implementaremos um serviço em Go com a seguinte arquitetura:

### 1. Consumo de Mensagens
- Tornar a etapa de ingestão de registros resiliente utilizando filas.
- Processar mensagens com base no MessageGroupId para determinar o banco de dados do cliente

### 2. Processamento de Dados
- Definir structs em Go correspondentes aos schemas FHIR
- Usar o driver MongoDB Go para persistência de dados
- Implementar integridade relacional armazenando referências entre recursos

### 3. Tratamento de Erros
- Logs abrangentes com rotação (retenção de 3 dias)
- Pular mensagens que não podem ser processadas (com registro)
- Reconexão automática para banco de dados e fila

### 4. Características Operacionais
- Configuração por variáveis de ambiente
- Design compatível com containers (logs para stdout e arquivos)

## Consequências:

### ✅ Positivas
- **Performance**: A eficiência do Go lida com alto throughput de mensagens
- **Confiabilidade**: Logs estruturados e tratamento de erros melhoram a observabilidade
- **Resiliencia**: Em caso de falhas, as mensagem serão reprocessadas ou irão para DLQ
- **Flexibilidade**: Fácil adição de novos tipos de recursos ou bancos de dados
- **Escalabilidade**: Solução escala horizontamente para lidar com grande volume de mensagens em paralelo.

### ❌ Negativas
- **Depuração**: Processamento assíncrono pode dificultar o tracing

### ➖ Neutras
- Requer alinhamento do schema MongoDB com recursos FHIR
- Assume formato específico de mensagem SQS e atributos


## Notas de Implementação:
A implementação atual:
- Implementa rotação de logs para higiene operacional
- Gerencia conexões de banco de dados eficientemente
- Processa mensagens em loop contínuo com recuperação de erros
- Pode ser utilizada com 2 ou mais instâncias do serviço para processamento paralelo das mensagens.


## Referências:
- [Especificação FHIR](https://www.hl7.org/fhir/)
- [AWS SQS Developer Guide](https://docs.aws.amazon.com/AWSSimpleQueueService/latest/SQSDeveloperGuide/welcome.html)
- [MongoDB Data Modeling Guidelines](https://www.mongodb.com/docs/manual/core/data-modeling-introduction/)