package types

import (
	"encoding/json"
	"fmt"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/x/sentinel/rest"
	"github.com/tendermint/tendermint/crypto"
	"github.com/tendermint/tendermint/libs/bech32"
)

type ClientSignature struct {
	Coins     sdk.Coins
	Sessionid []byte
	Counter   int64
	Signature rest.Signature
	IsFinal   bool
}

type SignatureA struct {
	Pubkey    crypto.PubKey    `json:"pub_key"`
	Signature rest.Signature `json:"signature"`
}

func NewClientSignature(coins sdk.Coins, sesid []byte, counter int64, pubkey crypto.PubKey, sign rest.Signature, isfinal bool) ClientSignature {
	return ClientSignature{
		Coins:     coins,
		Sessionid: sesid,
		Counter:   counter,
		IsFinal:   isfinal,
		Signature: SignatureA{
			Pubkey:    pubkey,
			Signature: sign,
		},
	}
}

func (a ClientSignature) Value() SignatureA {
	return a.Signature
}

type StdSig struct {
	Coins     sdk.Coins
	Sessionid []byte
	Counter   int64
	Isfinal   bool
}

func ClientStdSignBytes(coins sdk.Coins, sessionid []byte, counter int64, isfinal bool) []byte {
	bz, err := json.Marshal(StdSig{
		Coins:     coins,
		Sessionid: sessionid,
		Counter:   counter,
		Isfinal:   isfinal,
	})
	if err != nil {
	}
	return sdk.MustSortJSON(bz)
}

type Vpnsign struct {
	From     sdk.AccAddress
	Ip       string
	Netspeed int64
	Ppgb     int64
	Location string
}

func GetVPNSignature(address sdk.AccAddress, ip string, ppgb int64, netspeed int64, location string) []byte {
	bz, err := json.Marshal(Vpnsign{
		From:     address,
		Ip:       ip,
		Ppgb:     ppgb,
		Netspeed: netspeed,
		Location: location,
	})
	if err != nil {

	}
	return sdk.MustSortJSON(bz)
}
func GetBech32Signature(sign rest.Signature) (string, error) {
	return bech32.ConvertAndEncode("", sign.Bytes())

}

func GetBech64Signature(address string) (pk rest.Signature, err error) {
	hrp, bz, err := DecodeAndConvert(address)
	if err != nil {
		return nil, err
	}
	if hrp != "" {
		return nil, fmt.Errorf("invalid bech32 prefix. Expected %s, Got %s", "", hrp)
	}

	pk, err = rest.SignatureFromBytes(bz)

	if err != nil {
		return nil, err
	}

	return pk, nil
}

