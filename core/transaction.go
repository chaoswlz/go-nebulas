// Copyright (C) 2017 go-nebulas authors
//
// This file is part of the go-nebulas library.
//
// the go-nebulas library is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// the go-nebulas library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU General Public License for more details.
//
// You should have received a copy of the GNU General Public License
// along with the go-nebulas library.  If not, see <http://www.gnu.org/licenses/>.
//

package core

import (
	"errors"
	"time"

	"github.com/gogo/protobuf/proto"
	"github.com/nebulasio/go-nebulas/core/pb"
	"github.com/nebulasio/go-nebulas/crypto/cipher"
	"github.com/nebulasio/go-nebulas/crypto/hash"
	"github.com/nebulasio/go-nebulas/crypto/keystore"
	"github.com/nebulasio/go-nebulas/util/byteutils"
	log "github.com/sirupsen/logrus"
)

var (
	// ErrInsufficientBalance insufficient balance error.
	ErrInsufficientBalance = errors.New("insufficient balance")

	// ErrInvalidSignature the signature is not sign by from address.
	ErrInvalidSignature = errors.New("invalid transaction signature")

	// ErrInvalidTransactionHash invalid hash.
	ErrInvalidTransactionHash = errors.New("invalid transaction hash")

	// ErrFromAddressLocked from address locked.
	ErrFromAddressLocked = errors.New("from address locked")
)

// Transaction type is used to handle all transaction data.
type Transaction struct {
	hash      Hash
	from      Address
	to        Address
	value     uint64
	nonce     uint64
	timestamp time.Time
	data      []byte
	chainID   uint32

	// Signature
	alg  uint8 // algorithm
	sign Hash  // Signature values
}

// From return from address
func (tx *Transaction) From() []byte {
	return tx.from.address
}

// Nonce return tx nonce
func (tx *Transaction) Nonce() uint64 {
	return tx.nonce
}

// DataLen return data length
func (tx *Transaction) DataLen() int {
	return len(tx.data)
}

// ToProto converts domain Tx to proto Tx
func (tx *Transaction) ToProto() (proto.Message, error) {
	return &corepb.Transaction{
		Hash:      tx.hash,
		From:      tx.from.address,
		To:        tx.to.address,
		Value:     tx.value,
		Nonce:     tx.nonce,
		Timestamp: tx.timestamp.UnixNano(),
		Data:      tx.data,
		ChainID:   tx.chainID,
		Alg:       uint32(tx.alg),
		Sign:      tx.sign,
	}, nil
}

// FromProto converts proto Tx into domain Tx
func (tx *Transaction) FromProto(msg proto.Message) error {
	if msg, ok := msg.(*corepb.Transaction); ok {
		tx.hash = msg.Hash
		tx.from = Address{msg.From}
		tx.to = Address{msg.To}
		tx.value = msg.Value
		tx.nonce = msg.Nonce
		tx.timestamp = time.Unix(0, msg.Timestamp)
		tx.data = msg.Data
		tx.chainID = msg.ChainID
		tx.alg = uint8(msg.Alg)
		tx.sign = msg.Sign
		return nil
	}
	return errors.New("Pb Message cannot be converted into Transaction")
}

// Transactions is an alias of Transaction array.
type Transactions []*Transaction

// NewTransaction create #Transaction instance.
func NewTransaction(chainID uint32, from, to Address, value uint64, nonce uint64, data []byte) *Transaction {
	tx := &Transaction{
		from:      from,
		to:        to,
		value:     value,
		nonce:     nonce,
		timestamp: time.Now(),
		chainID:   chainID,
		data:      data,
	}
	return tx
}

// Hash return the hash of transaction.
func (tx *Transaction) Hash() Hash {
	return tx.hash
}

// Sign sign transaction.
func (tx *Transaction) Sign() error {
	tx.hash = HashTransaction(tx)
	key, err := keystore.DefaultKS.GetUnlocked(tx.from.ToHex())
	if err != nil {
		log.WithFields(log.Fields{
			"func": "Transaction.Sign",
			"err":  ErrInvalidTransactionHash,
			"tx":   tx,
		}).Error("from address locked")
		return err
	}
	alg := cipher.SECP256K1
	signature, err := cipher.GetSignature(alg)
	if err != nil {
		return err
	}
	signature.InitSign(key.(keystore.PrivateKey))
	sign, err := signature.Sign(tx.hash)
	if err != nil {
		return err
	}
	tx.alg = uint8(alg)
	tx.sign = sign
	return nil
}

// Verify return transaction verify result, including Hash and Signature.
func (tx *Transaction) Verify() error {
	wantedHash := HashTransaction(tx)
	if wantedHash.Equals(tx.hash) == false {
		log.WithFields(log.Fields{
			"func": "Transaction.Verify",
			"err":  ErrInvalidTransactionHash,
			"tx":   tx,
		}).Error("invalid transaction hash")
		return ErrInvalidTransactionHash
	}

	signVerify, err := tx.verifySign()
	if err != nil {
		return err
	}
	if !signVerify {
		return errors.New("verifySign failed")
	}
	return nil
}

// VerifySign verify the transaction sign
func (tx *Transaction) verifySign() (bool, error) {
	if len(tx.sign) == 0 {
		return false, errors.New("VerifySign need sign hash")
	}
	signature, err := cipher.GetSignature(cipher.Algorithm(tx.alg))
	if err != nil {
		return false, err
	}
	pub, err := signature.RecoverPublic(tx.hash, tx.sign)
	if err != nil {
		return false, err
	}
	pubdata, err := pub.Encoded()
	if err != nil {
		return false, err
	}
	addr, err := NewAddressFromPublicKey(pubdata)
	if err != nil {
		return false, err
	}
	if !tx.from.Equals(*addr) {
		return false, errors.New("recover public key not related to from address")
	}
	return signature.Verify(tx.hash, tx.sign)
}

// HashTransaction hash the transaction.
func HashTransaction(tx *Transaction) Hash {
	return hash.Sha3256(
		tx.from.address,
		tx.to.address,
		byteutils.FromUint64(tx.value),
		byteutils.FromUint64(tx.nonce),
		byteutils.FromInt64(tx.timestamp.UnixNano()),
		tx.data,
		byteutils.FromUint32(tx.chainID),
	)
}
