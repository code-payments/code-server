package jupiter

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/mr-tron/base58"
	"github.com/pkg/errors"

	"github.com/code-payments/code-server/pkg/metrics"
	"github.com/code-payments/code-server/pkg/solana"
)

// Reference: https://station.jup.ag/docs/apis/swap-api

const (
	DefaultApiBaseUrl = "https://quote-api.jup.ag/v6/"

	quoteEndpointName            = "quote"
	swapInstructionsEndpointName = "swap-instructions"

	metricsStructName = "jupiter.client"
)

type Client struct {
	baseUrl    string
	httpClient *http.Client
}

// NewClient returns a new Jupiter client for performing on-chain swaps
func NewClient(baseUrl string) *Client {
	return &Client{
		baseUrl:    baseUrl,
		httpClient: http.DefaultClient,
	}
}

type Quote struct {
	jsonString            string
	estimatedSwapAmount   uint64
	useSharedAccounts     bool
	useLegacyInstructions bool
}

func (q *Quote) GetEstimatedSwapAmount() uint64 {
	return q.estimatedSwapAmount
}

// GetQuote gets an optimal route for performing a swap
func (c *Client) GetQuote(
	ctx context.Context,
	inputMint string,
	outputMint string,
	quarksToSwap uint64,
	slippageBps uint32,
	forceDirectRoute bool,
	maxAccounts uint8,
	useSharedAccounts bool,
	useLegacyInstruction bool,
) (*Quote, error) {
	tracer := metrics.TraceMethodCall(ctx, metricsStructName, "GetQuote")
	defer tracer.End()

	url := fmt.Sprintf(
		"%s%s?inputMint=%s&outputMint=%s&amount=%d&slippageBps=%d&onlyDirectRoutes=%v&maxAccounts=%d&useSharedAccounts=%v&asLegacyTransaction=%v",
		c.baseUrl,
		quoteEndpointName,
		inputMint,
		outputMint,
		quarksToSwap,
		slippageBps,
		forceDirectRoute,
		maxAccounts,
		useSharedAccounts,
		useLegacyInstruction,
	)

	resp, err := c.httpClient.Get(url)
	if err != nil {
		return nil, errors.Wrap(err, "error executing http request")
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, errors.Wrap(err, "error reading response body")
	}

	if resp.StatusCode != http.StatusOK {
		return nil, errors.Errorf("received http status %d: %s", resp.StatusCode, string(respBody))
	}

	var parsed jsonQuote
	err = json.Unmarshal(respBody, &parsed)
	if err != nil {
		return nil, errors.Wrap(err, "error unmarshalling json response")
	}

	estimatedSwapAmount, err := strconv.ParseUint(parsed.OtherAmountThreshold, 10, 64)
	if err != nil {
		return nil, errors.Wrap(err, "error parsing estimated swap amount")
	}

	return &Quote{
		jsonString:            string(respBody),
		estimatedSwapAmount:   estimatedSwapAmount,
		useSharedAccounts:     useSharedAccounts,
		useLegacyInstructions: useLegacyInstruction,
	}, nil
}

type SwapInstructions struct {
	TokenLedgerInstruction    *solana.Instruction
	ComputeBudgetInstructions []solana.Instruction
	SetupInstructions         []solana.Instruction
	SwapInstruction           solana.Instruction
	CleanupInstruction        *solana.Instruction
}

// GetSwapInstructions gets the instructions to construct a transaction to sign
// and execute on chain to perform a swap with a given quote
func (c *Client) GetSwapInstructions(
	ctx context.Context,
	quote *Quote,
	owner string,
	destinationTokenAccount string,
) (*SwapInstructions, error) {
	tracer := metrics.TraceMethodCall(ctx, metricsStructName, "GetSwapInstructions")
	defer tracer.End()

	if !quote.useLegacyInstructions {
		return nil, errors.New("only legacy transactions are supported")
	}

	// todo: struct this
	reqBody := fmt.Sprintf(
		`{"quoteResponse": %s, "userPublicKey": "%s", "destinationTokenAccount": "%s", "prioritizationFeeLamports": "auto", "useSharedAccounts": %v, "asLegacyTransaction": %v}`,
		quote.jsonString,
		owner,
		destinationTokenAccount,
		quote.useSharedAccounts,
		quote.useLegacyInstructions,
	)

	resp, err := http.Post(c.baseUrl+swapInstructionsEndpointName, "application/json", strings.NewReader(reqBody))
	if err != nil {
		return nil, errors.Wrap(err, "error executing http request")
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, errors.Wrap(err, "error reading response body")
	}

	if resp.StatusCode != http.StatusOK {
		return nil, errors.Errorf("received http status %d: %s", resp.StatusCode, string(respBody))
	}

	var jsonBody jsonSwapInstructions
	err = json.Unmarshal(respBody, &jsonBody)
	if err != nil {
		return nil, errors.Wrap(err, "error unmarshalling json response")
	}

	var res SwapInstructions

	if res.TokenLedgerInstruction != nil {
		res.TokenLedgerInstruction, err = jsonBody.TokenLedgerInstruction.ToSolanaInstruction()
		if err != nil {
			return nil, errors.Wrap(err, "error decoding token ledger instruction")
		}
	}

	for _, jsonIxn := range jsonBody.ComputeBudgetInstructions {
		cbIxn, err := jsonIxn.ToSolanaInstruction()
		if err != nil {
			return nil, errors.Wrap(err, "error decoding compute budget instruction")
		}
		res.ComputeBudgetInstructions = append(res.ComputeBudgetInstructions, *cbIxn)
	}

	for _, jsonIxn := range jsonBody.SetupInstructions {
		setupIxn, err := jsonIxn.ToSolanaInstruction()
		if err != nil {
			return nil, errors.Wrap(err, "error decoding setup instruction")
		}
		res.SetupInstructions = append(res.SetupInstructions, *setupIxn)
	}

	if jsonBody.SwapInstruction == nil {
		return nil, errors.New("swap instruction not provided")
	}

	swapIxn, err := jsonBody.SwapInstruction.ToSolanaInstruction()
	if err != nil {
		return nil, errors.Wrap(err, "error decoding swap instruction")
	}
	res.SwapInstruction = *swapIxn

	if res.CleanupInstruction != nil {
		res.CleanupInstruction, err = jsonBody.CleanupInstruction.ToSolanaInstruction()
		if err != nil {
			return nil, errors.Wrap(err, "error decoding cleanup instruction")
		}
	}

	return &res, nil
}

func (i *jsonInstruction) ToSolanaInstruction() (*solana.Instruction, error) {
	decodedProgramKey, err := base58.Decode(i.ProgramId)
	if err != nil {
		return nil, errors.Wrap(err, "invalid program public key")
	}

	decodedData, err := base64.StdEncoding.DecodeString(i.Data)
	if err != nil {
		return nil, errors.Wrap(err, "error decoding base64 instruction data")
	}

	var accountMetas []solana.AccountMeta
	for _, instructionAccount := range i.Accounts {
		decodedPubkey, err := base58.Decode(instructionAccount.Pubkey)
		if err != nil {
			return nil, errors.Wrap(err, "invalid instruction account public key")
		}

		accountMetas = append(accountMetas, solana.AccountMeta{
			PublicKey:  decodedPubkey,
			IsSigner:   instructionAccount.IsSigner,
			IsWritable: instructionAccount.IsWritable,
		})
	}

	return &solana.Instruction{
		Program:  decodedProgramKey,
		Accounts: accountMetas,
		Data:     decodedData,
	}, nil
}

type jsonQuote struct {
	OtherAmountThreshold string `json:"otherAmountThreshold"`
}

type jsonInstructionAccount struct {
	Pubkey     string `json:"pubkey"`
	IsSigner   bool   `json:"isSigner"`
	IsWritable bool   `json:"isWritable"`
}

type jsonInstruction struct {
	ProgramId string                   `json:"programId"`
	Accounts  []jsonInstructionAccount `json:"accounts"`
	Data      string                   `json:"data"`
}

type jsonSwapInstructions struct {
	TokenLedgerInstruction    *jsonInstruction   `json:"tokenLedgerInstruction"`
	ComputeBudgetInstructions []*jsonInstruction `json:"computeBudgetInstructions"`
	SetupInstructions         []*jsonInstruction `json:"setupInstructions"`
	SwapInstruction           *jsonInstruction   `json:"swapInstruction"`
	CleanupInstruction        *jsonInstruction   `json:"cleanupInstruction"`
}
