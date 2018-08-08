package slashing

import (
	"fmt"
	"time"

	"github.com/tendermint/tendermint/crypto/tmhash"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/wire"
	"github.com/cosmos/cosmos-sdk/x/params"
	"github.com/tendermint/tendermint/crypto"
)

// Keeper of the slashing store
type Keeper struct {
	storeKey        sdk.StoreKey
	cdc             *wire.Codec
	validatorSet    sdk.ValidatorSet
	params          params.Getter
	addressToPubkey map[[tmhash.Size]byte]crypto.PubKey
	// codespace
	codespace sdk.CodespaceType
}

// NewKeeper creates a slashing keeper
func NewKeeper(cdc *wire.Codec, key sdk.StoreKey, vs sdk.ValidatorSet, params params.Getter, codespace sdk.CodespaceType) Keeper {
	keeper := Keeper{
		storeKey:     key,
		cdc:          cdc,
		validatorSet: vs,
		params:       params,
		codespace:    codespace,
	}
	return keeper
}

// handle a validator signing two blocks at the same height
func (k Keeper) handleDoubleSign(ctx sdk.Context, pubkey crypto.PubKey, infractionHeight int64, timestamp time.Time, power int64) {
	logger := ctx.Logger().With("module", "x/slashing")
	time := ctx.BlockHeader().Time
	age := time.Sub(timestamp)
	address := sdk.ValAddress(pubkey.Address())

	// Double sign too old
	maxEvidenceAge := k.MaxEvidenceAge(ctx)
	if age > maxEvidenceAge {
		logger.Info(fmt.Sprintf("Ignored double sign from %s at height %d, age of %d past max age of %d", pubkey.Address(), infractionHeight, age, maxEvidenceAge))
		return
	}

	// Double sign confirmed
	logger.Info(fmt.Sprintf("Confirmed double sign from %s at height %d, age of %d less than max age of %d", pubkey.Address(), infractionHeight, age, maxEvidenceAge))

	// Slash validator
	k.validatorSet.Slash(ctx, pubkey, infractionHeight, power, k.SlashFractionDoubleSign(ctx))

	// Revoke validator
	k.validatorSet.Revoke(ctx, pubkey)

	// Jail validator
	signInfo, found := k.getValidatorSigningInfo(ctx, address)
	if !found {
		panic(fmt.Sprintf("Expected signing info for validator %s but not found", address))
	}
	signInfo.JailedUntil = time.Add(k.DoubleSignUnbondDuration(ctx))
	k.setValidatorSigningInfo(ctx, address, signInfo)
}

// handle a validator signature, must be called once per validator per block
// nolint gocyclo
// TODO: Change this to take in an address
func (k Keeper) handleValidatorSignature(ctx sdk.Context, pubkey crypto.PubKey, power int64, signed bool) {
	logger := ctx.Logger().With("module", "x/slashing")
	height := ctx.BlockHeight()
	address := sdk.ValAddress(pubkey.Address())

	// Local index, so counts blocks validator *should* have signed
	// Will use the 0-value default signing info if not present, except for start height
	signInfo, found := k.getValidatorSigningInfo(ctx, address)
	if !found {
		// If this validator has never been seen before, construct a new SigningInfo with the correct start height
		signInfo = NewValidatorSigningInfo(height, 0, time.Unix(0, 0), 0)
	}
	index := signInfo.IndexOffset % k.SignedBlocksWindow(ctx)
	signInfo.IndexOffset++

	// Update signed block bit array & counter
	// This counter just tracks the sum of the bit array
	// That way we avoid needing to read/write the whole array each time
	previous := k.getValidatorSigningBitArray(ctx, address, index)
	if previous == signed {
		// Array value at this index has not changed, no need to update counter
	} else if previous && !signed {
		// Array value has changed from signed to unsigned, decrement counter
		k.setValidatorSigningBitArray(ctx, address, index, false)
		signInfo.SignedBlocksCounter--
	} else if !previous && signed {
		// Array value has changed from unsigned to signed, increment counter
		k.setValidatorSigningBitArray(ctx, address, index, true)
		signInfo.SignedBlocksCounter++
	}

	if !signed {
		logger.Info(fmt.Sprintf("Absent validator %s at height %d, %d signed, threshold %d", pubkey.Address(), height, signInfo.SignedBlocksCounter, k.MinSignedPerWindow(ctx)))
	}
	minHeight := signInfo.StartHeight + k.SignedBlocksWindow(ctx)
	if height > minHeight && signInfo.SignedBlocksCounter < k.MinSignedPerWindow(ctx) {
		validator := k.validatorSet.ValidatorByPubKey(ctx, pubkey)
		if validator != nil && !validator.GetRevoked() {
			// Downtime confirmed, slash, revoke, and jail the validator
			logger.Info(fmt.Sprintf("Validator %s past min height of %d and below signed blocks threshold of %d",
				pubkey.Address(), minHeight, k.MinSignedPerWindow(ctx)))
			k.validatorSet.Slash(ctx, pubkey, height, power, k.SlashFractionDowntime(ctx))
			k.validatorSet.Revoke(ctx, pubkey)
			signInfo.JailedUntil = ctx.BlockHeader().Time.Add(k.DowntimeUnbondDuration(ctx))
		} else {
			// Validator was (a) not found or (b) already revoked, don't slash
			logger.Info(fmt.Sprintf("Validator %s would have been slashed for downtime, but was either not found in store or already revoked",
				pubkey.Address()))
		}
	}

	// Set the updated signing info
	k.setValidatorSigningInfo(ctx, address, signInfo)
}

func (k Keeper) addValidatorAddress(addrSlice []byte, key crypto.PubKey) {
	if len(addrSlice) != tmhash.Size {
		return
	}
	addr := new([tmhash.Size]byte)
	copy(addr[:], addrSlice)
	k.addressToPubkey[*addr] = key
}
