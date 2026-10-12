package plugin

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/luode0320/cpa-workbuddy-plugin/cursor/internal/cursorapi"
	"github.com/luode0320/cpa-workbuddy-plugin/cursor/internal/cursorauth"
	"github.com/luode0320/cpa-workbuddy-plugin/cursor/internal/cursorproto"

	"github.com/stretchr/testify/require"
)

func Test_Handler_ModelsForAuth_hides_models_disabled_by_cursor_plugin(t *testing.T) {
	handler := NewHandler(Dependencies{Cursor: fakeModelCursorClient{models: []string{"auto", "claude-4-sonnet", "gpt-5"}}})
	storage, err := cursorauth.MarshalCredentials(cursorauth.Credentials{
		AccessToken:    "access",
		RefreshToken:   "refresh",
		Type:           "cursor-provider",
		DisabledModels: []string{"cursor/gpt-5"},
	})
	require.NoError(t, err)
	rawRequest, err := json.Marshal(authModelRequest{StorageJSON: storage})
	require.NoError(t, err)

	result, err := handler.modelsForAuth(context.Background(), rawRequest)

	require.NoError(t, err)
	rawResult, err := json.Marshal(result)
	require.NoError(t, err)
	require.Contains(t, string(rawResult), `"ID":"cursor/auto"`)
	require.Contains(t, string(rawResult), `"SupportedInputModalities":["text","image"]`)
	require.Contains(t, string(rawResult), `"SupportedOutputModalities":["text","image"]`)
	require.Contains(t, string(rawResult), `"ID":"cursor/claude-4-sonnet"`)
	require.NotContains(t, string(rawResult), `"ID":"cursor/gpt-5"`)
}

func Test_Handler_static_models_omit_unverified_token_limits(t *testing.T) {
	// Given
	handler := NewHandler(Dependencies{Cursor: fakeModelCursorClient{}})

	// When
	raw, ok := handler.CallWithStatus(context.Background(), "model.static", nil)

	// Then
	require.True(t, ok)
	var response struct {
		Result struct {
			Models []struct {
				ContextLength       *int64
				MaxCompletionTokens *int64
			}
		}
	}
	require.NoError(t, json.Unmarshal(raw, &response))
	require.Len(t, response.Result.Models, len(defaultCursorModels))
	for _, m := range response.Result.Models {
		require.Nil(t, m.ContextLength)
		require.Nil(t, m.MaxCompletionTokens)
	}
}

func Test_Handler_static_models_discovers_models_from_available_credential(t *testing.T) {
	// 模拟已导入凭据
	credentials, err := cursorauth.MarshalCredentials(cursorauth.Credentials{
		AccessToken:  "valid-access-token",
		RefreshToken: "refresh-token",
		Type:         "cursor-provider",
	})
	require.NoError(t, err)

	host := &fakeHostCaller{
		credentialJSON: credentials,
		listJSON: json.RawMessage(`{"files":[{
			"id":"cursor-acc-1",
			"auth_index":"cursor-auth-index-1",
			"name":"cursor-acc-1.json",
			"type":"cursor-provider"
		}]}`),
	}
	client := fakeModelCursorClient{models: []string{"custom-model-1", "custom-model-2"}}
	handler := NewHandler(Dependencies{Cursor: client, Host: host})

	raw, ok := handler.CallWithStatus(context.Background(), "model.static", nil)
	require.True(t, ok)

	var response struct {
		Result struct {
			Provider string
			Models   []struct {
				ID          string
				DisplayName string
			}
		}
	}
	require.NoError(t, json.Unmarshal(raw, &response))
	require.Equal(t, "cursor-provider", response.Result.Provider)
	require.Len(t, response.Result.Models, 2)
	require.Equal(t, "cursor/custom-model-1", response.Result.Models[0].ID)
	require.Equal(t, "cursor/custom-model-2", response.Result.Models[1].ID)
}

type fakeModelCursorClient struct {
	models []string
}

func (client fakeModelCursorClient) Run(context.Context, cursorapi.RunInput, func(cursorproto.ServerEvent) error) (cursorapi.RunResult, error) {
	return cursorapi.RunResult{}, nil
}

func (client fakeModelCursorClient) DiscoverModels(context.Context, string) ([]string, error) {
	return append([]string(nil), client.models...), nil
}
