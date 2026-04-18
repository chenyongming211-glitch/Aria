package handlers

import (
	"aria/pkg/controllerstorage"
)

type TenantManagementAPI struct {
	store *controllerstorage.Storage
}

func NewTenantManagementAPI(store *controllerstorage.Storage) *TenantManagementAPI {
	return &TenantManagementAPI{
		store: store,
	}
}
