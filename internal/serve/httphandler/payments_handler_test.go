package httphandler

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi"
	"github.com/stellar/go/keypair"
	"github.com/stellar/wallet-backend/internal/data"
	"github.com/stellar/wallet-backend/internal/db"
	"github.com/stellar/wallet-backend/internal/db/dbtest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSubscribeAddress(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()

	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	models, err := data.NewModels(dbConnectionPool)
	require.NoError(t, err)
	handler := &PaymentsHandler{
		Models: models,
	}

	// Setup router
	r := chi.NewRouter()
	r.Post("/payments/subscribe", handler.SubscribeAddress)

	clearAccounts := func(ctx context.Context) {
		_, err = dbConnectionPool.ExecContext(ctx, "TRUNCATE accounts")
		require.NoError(t, err)
	}

	t.Run("success_happy_path", func(t *testing.T) {
		// Prepare request
		address := keypair.MustRandom().Address()
		payload := fmt.Sprintf(`{ "address": %q }`, address)
		req, err := http.NewRequest(http.MethodPost, "/payments/subscribe", strings.NewReader(payload))
		require.NoError(t, err)

		// Serve request
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)

		// Assert 200 response
		assert.Equal(t, http.StatusOK, rr.Code)

		ctx := context.Background()
		var dbAddress sql.NullString
		err = dbConnectionPool.GetContext(ctx, &dbAddress, "SELECT stellar_address FROM accounts")
		require.NoError(t, err)

		// Assert address persisted in DB
		assert.True(t, dbAddress.Valid)
		assert.Equal(t, address, dbAddress.String)

		clearAccounts(ctx)
	})

	t.Run("address_already_exists", func(t *testing.T) {
		address := keypair.MustRandom().Address()
		ctx := context.Background()

		// Insert address in DB
		_, err = dbConnectionPool.ExecContext(ctx, "INSERT INTO accounts (stellar_address) VALUES ($1)", address)
		require.NoError(t, err)

		// Prepare request
		payload := fmt.Sprintf(`{ "address": %q }`, address)
		req, err := http.NewRequest(http.MethodPost, "/payments/subscribe", strings.NewReader(payload))
		require.NoError(t, err)

		// Serve request
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)

		// Assert 200 response
		assert.Equal(t, http.StatusOK, rr.Code)

		var dbAddress sql.NullString
		err = dbConnectionPool.GetContext(ctx, &dbAddress, "SELECT stellar_address FROM accounts")
		require.NoError(t, err)

		// Assert address persisted in DB
		assert.True(t, dbAddress.Valid)
		assert.Equal(t, address, dbAddress.String)

		clearAccounts(ctx)
	})

	t.Run("invalid_address", func(t *testing.T) {
		// Prepare request
		payload := fmt.Sprintf(`{ "address": %q }`, "invalid")
		req, err := http.NewRequest(http.MethodPost, "/payments/subscribe", strings.NewReader(payload))
		require.NoError(t, err)

		// Serve request
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)

		resp := rr.Result()
		respBody, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
		assert.JSONEq(t, `{"error":"Validation error.", "extras": {"address":"Invalid public key provided"}}`, string(respBody))
	})
}

func TestUnsubscribeAddress(t *testing.T) {
	dbt := dbtest.Open(t)
	defer dbt.Close()

	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	models, err := data.NewModels(dbConnectionPool)
	require.NoError(t, err)
	handler := &PaymentsHandler{
		Models: models,
	}

	// Setup router
	r := chi.NewRouter()
	r.Post("/payments/unsubscribe", handler.UnsubscribeAddress)

	t.Run("successHappyPath", func(t *testing.T) {
		address := keypair.MustRandom().Address()
		ctx := context.Background()

		// Insert address in DB
		_, err = dbConnectionPool.ExecContext(ctx, "INSERT INTO accounts (stellar_address) VALUES ($1)", address)
		require.NoError(t, err)

		// Prepare request
		payload := fmt.Sprintf(`{ "address": %q }`, address)
		req, err := http.NewRequest(http.MethodPost, "/payments/unsubscribe", strings.NewReader(payload))
		require.NoError(t, err)

		// Serve request
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)

		// Assert 200 response
		assert.Equal(t, http.StatusOK, rr.Code)

		// Assert no address no longer in DB
		var dbAddress sql.NullString
		err = dbConnectionPool.GetContext(ctx, &dbAddress, "SELECT stellar_address FROM accounts")
		assert.ErrorIs(t, err, sql.ErrNoRows)
	})

	t.Run("idempotency", func(t *testing.T) {
		address := keypair.MustRandom().Address()
		ctx := context.Background()

		// Make sure DB is empty
		_, err = dbConnectionPool.ExecContext(ctx, "DELETE FROM accounts")
		require.NoError(t, err)

		// Prepare request
		payload := fmt.Sprintf(`{ "address": %q }`, address)
		req, err := http.NewRequest(http.MethodPost, "/payments/unsubscribe", strings.NewReader(payload))
		require.NoError(t, err)

		// Serve request
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)

		// Assert 200 response
		assert.Equal(t, http.StatusOK, rr.Code)
	})

	t.Run("invalid_address", func(t *testing.T) {
		// Prepare request
		payload := fmt.Sprintf(`{ "address": %q }`, "invalid")
		req, err := http.NewRequest(http.MethodPost, "/payments/unsubscribe", strings.NewReader(payload))
		require.NoError(t, err)

		// Serve request
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)

		resp := rr.Result()
		respBody, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
		assert.JSONEq(t, `{"error":"Validation error.", "extras": {"address":"Invalid public key provided"}}`, string(respBody))
	})
}
