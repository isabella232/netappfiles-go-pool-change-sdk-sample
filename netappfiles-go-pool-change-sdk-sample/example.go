// Copyright (c) Microsoft and contributors.  All rights reserved.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

// This sample code shows how to enable cross-region replication
// on an NFSv3 volume by creating primary and secondary resources
// (Account, Capacity Pool, Volumes), then enabling it from primary
// volume. Clean up process (not enabled by default) is made in
// reverse order, but it starts by deleting the data replication object
// from secondary volume. Clean up process is not taking place if
// there is an execution failure, you will need to clean it up manually
// in this case.

package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/Azure-Samples/netappfiles-go-pool-change-sdk-sample/netappfiles-go-pool-change-sdk-sample/internal/sdkutils"
	"github.com/Azure-Samples/netappfiles-go-pool-change-sdk-sample/netappfiles-go-pool-change-sdk-sample/internal/utils"
	"github.com/Azure/azure-sdk-for-go/profiles/latest/netapp/mgmt/netapp"
	"github.com/Azure/go-autorest/autorest/to"
	"github.com/yelinaung/go-haikunator"
)

const (
	virtualNetworksAPIVersion string = "2019-09-01"
)

type (
	// Properties - properties to be used when defining primary and secondary ANF resources
	Properties struct {
		CapacityPoolName string
		ServiceLevel     string // Valid service levels are Standard, Premium and Ultra
		CapacityPoolID   string // This will be populated after resource is created
	}
)

var (
	shouldCleanUp bool = false

	// Important - change ANF related variables below to appropriate values related to your environment
	// Share ANF properties related
	location                    = "eastus"
	resourceGroupName           = "ANF01-rg"
	vnetresourceGroupName       = "ANF01-rg"
	vnetName                    = "vnet-01"
	subnetName                  = "ANF-sn"
	anfAccountName              = haikunator.New(time.Now().UTC().UnixNano()).Haikunate()
	volumeName                  = "NFSv3Volume01"
	capacityPoolSizeBytes int64 = 4398046511104 // 4TiB (minimum capacity pool size)
	volumeSizeBytes       int64 = 107374182400  // 100GiB (minimum volume size)
	protocolTypes               = []string{"NFSv3"}
	sampleTags                  = map[string]*string{
		"Author":  to.StringPtr("ANF Go Pool Change SDK Sample"),
		"Service": to.StringPtr("Azure Netapp Files"),
	}

	// Capacity Pools info
	pools = map[string]*Properties{
		"Source": {
			CapacityPoolName: "SourcePremiumPool",
			ServiceLevel:     "Premium",
		},
		"Destination": {
			CapacityPoolName: "DestinationStandardPool",
			ServiceLevel:     "Standard",
		},
	}

	// Some other variables used throughout the course of the code execution - no need to change it
	exitCode  int
	volumeID  string
	accountID string
)

func main() {

	cntx := context.Background()

	// Cleanup and exit handling
	defer func() { exit(cntx); os.Exit(exitCode) }()

	utils.PrintHeader("Azure NetAppFiles Go Pool Change SDK Sample - Sample application that changes an NFSv3 volume tier from Premium to Standard by moving it to a new Capacity Pool.")

	// Getting subscription ID from authentication file
	config, err := utils.ReadAzureBasicInfoJSON(os.Getenv("AZURE_AUTH_LOCATION"))
	if err != nil {
		utils.ConsoleOutput(fmt.Sprintf("an error ocurred getting non-sensitive info from AzureAuthFile: %v", err))
		exitCode = 1
		shouldCleanUp = false
		return
	}

	//------------------
	// Subnet validation
	//------------------

	// Checking if subnet exists before any other operation starts
	subnetID := fmt.Sprintf("/subscriptions/%v/resourceGroups/%v/providers/Microsoft.Network/virtualNetworks/%v/subnets/%v",
		*config.SubscriptionID,
		vnetresourceGroupName,
		vnetName,
		subnetName,
	)

	utils.ConsoleOutput(fmt.Sprintf("Checking if vnet/subnet %v exists.", subnetID))

	_, err = sdkutils.GetResourceByID(cntx, subnetID, virtualNetworksAPIVersion)
	if err != nil {
		if string(err.Error()) == "NotFound" {
			utils.ConsoleOutput(fmt.Sprintf("error: subnet %v not found: %v", subnetID, err))
		} else {
			utils.ConsoleOutput(fmt.Sprintf("error: an error ocurred trying to check if %v subnet exists: %v", subnetID, err))
		}
		exitCode = 1
		shouldCleanUp = false
		return
	}

	//------------------
	// Account creation
	//------------------
	utils.ConsoleOutput(fmt.Sprintf("Creating Azure NetApp Files account %v...", anfAccountName))

	account, err := sdkutils.CreateANFAccount(cntx, location, resourceGroupName, anfAccountName, nil, sampleTags)
	if err != nil {
		utils.ConsoleOutput(fmt.Sprintf("an error ocurred while creating account: %v", err))
		exitCode = 1
		shouldCleanUp = false
		return
	}
	accountID = *account.ID
	utils.ConsoleOutput(fmt.Sprintf("Account successfully created, resource id: %v", accountID))

	//------------------------
	// Creating Capacity Pools
	//------------------------
	for _, pool := range pools {
		// Capacity pool creation
		utils.ConsoleOutput(fmt.Sprintf("Creating %v Capacity Pool...", pool.CapacityPoolName))
		capacityPool, err := sdkutils.CreateANFCapacityPool(
			cntx,
			location,
			resourceGroupName,
			anfAccountName,
			pool.CapacityPoolName,
			pool.ServiceLevel,
			capacityPoolSizeBytes,
			sampleTags,
		)
		if err != nil {
			utils.ConsoleOutput(fmt.Sprintf("an error ocurred while creating %v capacity pool: %v", pool.CapacityPoolName, err))
			exitCode = 1
			shouldCleanUp = false
			return
		}
		pool.CapacityPoolID = *capacityPool.ID
		utils.ConsoleOutput(fmt.Sprintf("Capacity Pool successfully created, resource id: %v", pool.CapacityPoolID))
	}

	//----------------
	// Volume creation
	//----------------
	utils.ConsoleOutput(fmt.Sprintf("Creating NFSv3 Volume %v at %v capacity pool...", volumeName, pools["Source"].CapacityPoolName))

	volume, err := sdkutils.CreateANFVolume(
		cntx,
		location,
		resourceGroupName,
		anfAccountName,
		pools["Source"].CapacityPoolName,
		volumeName,
		pools["Source"].ServiceLevel,
		subnetID,
		"",
		protocolTypes,
		volumeSizeBytes,
		false,
		true,
		sampleTags,
		netapp.VolumePropertiesDataProtection{}, // This empty object is provided as nil since dataprotection is not scope of this sample
	)

	if err != nil {
		utils.ConsoleOutput(fmt.Sprintf("an error ocurred while creating volume: %v", err))
		exitCode = 1
		shouldCleanUp = false
		return
	}

	volumeID = *volume.ID
	utils.ConsoleOutput(fmt.Sprintf("Volume successfully created, resource id: %v", volumeID))

	utils.ConsoleOutput("Waiting for volume to be ready...")
	err = sdkutils.WaitForANFResource(cntx, volumeID, 60, 50, false)
	if err != nil {
		utils.ConsoleOutput(fmt.Sprintf("an error ocurred while waiting for volume: %v", err))
		exitCode = 1
		shouldCleanUp = false
		return
	}

	//---------------------------------------------
	// Moving Volume to Standard tier capacity pool
	//---------------------------------------------
	utils.ConsoleOutput(fmt.Sprintf("Moving Volume %v to %v capacity pool...", volumeName, pools["Destination"].CapacityPoolName))
	destinationPoolBody := netapp.PoolChangeRequest{
		NewPoolResourceID: &pools["Destination"].CapacityPoolID,
	}

	err = sdkutils.MoveANFVolumeToNewPool(
		cntx,
		resourceGroupName,
		anfAccountName,
		pools["Source"].CapacityPoolName,
		volumeName,
		destinationPoolBody,
	)

	if err != nil {
		utils.ConsoleOutput(fmt.Sprintf("an error ocurred while moving volume to another capacity pool: %v", err))
		exitCode = 1
		shouldCleanUp = false
		return
	}

	utils.ConsoleOutput("Wait a few seconds for move complete before deleting resources...")
	time.Sleep(time.Duration(5) * time.Second)
}

func exit(cntx context.Context) {
	utils.ConsoleOutput("Exiting")

	if shouldCleanUp {
		utils.ConsoleOutput("\tPerforming clean up")

		// Volume Cleanup
		utils.ConsoleOutput("\tCleaning up volume...")
		err := sdkutils.DeleteANFVolume(
			cntx,
			resourceGroupName,
			anfAccountName,
			pools["Destination"].CapacityPoolName,
			volumeName,
		)
		if err != nil {
			utils.ConsoleOutput(fmt.Sprintf("an error ocurred while deleting volume: %v", err))
			exitCode = 1
			return
		}
		sdkutils.WaitForNoANFResource(cntx, volumeID, 60, 60, false)
		utils.ConsoleOutput("\tVolume successfully deleted")

		// Capacity Pools Cleanup
		utils.ConsoleOutput("\tCleaning up capacity pools...")
		for _, pool := range pools {
			utils.ConsoleOutput(fmt.Sprintf("\t\tCleaning up %v...", pool.CapacityPoolName))
			err = sdkutils.DeleteANFCapacityPool(
				cntx,
				resourceGroupName,
				anfAccountName,
				pool.CapacityPoolName,
			)
			if err != nil {
				utils.ConsoleOutput(fmt.Sprintf("an error ocurred while deleting capacity pool: %v", err))
				exitCode = 1
				return
			}
			sdkutils.WaitForNoANFResource(cntx, pool.CapacityPoolID, 10, 60, false)
			utils.ConsoleOutput("\t\tCapacity pool successfully deleted")
		}
		utils.ConsoleOutput("\tCapacity pools successfully deleted")

		// Account Cleanup
		utils.ConsoleOutput("\tCleaning up account...")
		err = sdkutils.DeleteANFAccount(
			cntx,
			resourceGroupName,
			anfAccountName,
		)
		if err != nil {
			utils.ConsoleOutput(fmt.Sprintf("an error ocurred while deleting account: %v", err))
			exitCode = 1
			return
		}
		utils.ConsoleOutput("\tAccount successfully deleted")
		utils.ConsoleOutput("\tCleanup completed!")
	}
}
