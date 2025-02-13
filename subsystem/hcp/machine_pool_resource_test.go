/*
Copyright (c) 2021 Red Hat, Inc.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

  http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package hcp

import (
	"net/http"
	"strings"

	. "github.com/onsi/ginkgo/v2/dsl/core" // nolint
	. "github.com/onsi/gomega"             // nolint
	. "github.com/onsi/gomega/ghttp"       // nolint
	cmv1 "github.com/openshift-online/ocm-sdk-go/clustersmgmt/v1"
	. "github.com/openshift-online/ocm-sdk-go/testing" // nolint
)

const (
	cluster123Uri     = "/api/clusters_mgmt/v1/clusters/123"
	workerNodePoolUri = cluster123Uri + "/node_pools/worker"
)

var _ = Describe("Hcp Machine pool", func() {
	Context("static validation", func() {
		It("is invalid to specify both availability_zone and subnet_id", func() {
			terraform.Source(`
			resource "rhcs_hcp_machine_pool" "my_pool" {
				cluster      = "123"
				name         = "my-pool"
				aws_node_pool = {
					instance_type = "r5.xlarge"
				}
				replicas     = 12
				subnet_id = "subnet-123"
			}`)
			Expect(terraform.Validate()).NotTo(BeZero())
		})

		It("is necessary to specify both min and max replicas", func() {
			terraform.Source(`
			resource "rhcs_hcp_machine_pool" "my_pool" {
				cluster      = "123"
				name         = "my-pool"
				aws_node_pool = {
					instance_type = "r5.xlarge",
				}
				autoscaling = {
					enabled = true,
					min_replicas = 1
				}
				subnet_id = "subnet-123"
			}`)
			Expect(terraform.Validate()).NotTo(BeZero())

			terraform.Source(`
			resource "rhcs_hcp_machine_pool" "my_pool" {
				cluster      = "123"
				name         = "my-pool"
				aws_node_pool = {
					instance_type = "r5.xlarge",
				}
				autoscaling = {
					enabled = true,
					max_replicas = 5
				}
				subnet_id = "subnet-123"
			}`)
			Expect(terraform.Validate()).NotTo(BeZero())
		})

		It("is invalid to specify min_replicas and replicas", func() {
			terraform.Source(`
			resource "rhcs_hcp_machine_pool" "my_pool" {
				cluster      = "123"
				name         = "my-pool"
				aws_node_pool = {
					instance_type = "r5.xlarge",
				}
				autoscaling = {
					enabled = true,
					min_replicas = 1
				}
				replicas     = 5
				subnet_id = "subnet-123"
			}`)
			Expect(terraform.Validate()).NotTo(BeZero())
		})

		It("is invalid to specify max_replicas and replicas", func() {
			terraform.Source(`
			resource "rhcs_hcp_machine_pool" "my_pool" {
				cluster = "123"
				name = "my-pool"
				aws_node_pool = {
					instance_type = "r5.xlarge",
				}
				autoscaling = {
					enabled = true,
					max_replicas = 1
				}
				replicas = 5
				subnet_id = "subnet-123"
			}`)
			Expect(terraform.Validate()).NotTo(BeZero())
		})
	})

	Context("create", func() {
		BeforeEach(func() {
			// The first thing that the provider will do for any operation on machine pools
			// is check that the cluster is ready, so we always need to prepare the server to
			// respond to that:
			server.AppendHandlers(
				CombineHandlers(
					VerifyRequest(http.MethodGet, cluster123Uri),
					RespondWithJSON(http.StatusOK, `{
					  "id": "123",
					  "name": "my-cluster",
					  "multi_az": true,
					  "nodes": {
						"availability_zones": [
						  "us-east-1a",
						  "us-east-1b",
						  "us-east-1c"
						]
					  },
					  "state": "ready"
					}`),
				),
			)
		})

		It("Can create machine pool with compute nodes", func() {
			// Prepare the server:
			server.AppendHandlers(
				CombineHandlers(
					VerifyRequest(
						http.MethodPost,
						"/api/clusters_mgmt/v1/clusters/123/node_pools",
					),
					RespondWithJSON(http.StatusCreated, `{
					"id":"my-pool",
					"aws_node_pool":{
					   "instance_type":"r5.xlarge",
					   "instance_profile": "bla"
					},
					"auto_repair": true,
					"replicas":12,
					"labels":{
					   "label_key1":"label_value1",
					   "label_key2":"label_value2"
					},
					"subnet":"id-1",
					"availability_zone":"us-east-1a",
					"taints":[
					   {
						  "effect":"NoSchedule",
						  "key":"key1",
						  "value":"value1"
					   }
					],
					"version": {
						"raw_id": "4.14.10"
					}
				}`),
				),
			)

			// Run the apply command:
			terraform.Source(`
			resource "rhcs_hcp_machine_pool" "my_pool" {
				cluster      = "123"
				name         = "my-pool"
				aws_node_pool = {
					instance_type = "r5.xlarge",
				}
				autoscaling = {
					enabled = false,
				}
				subnet_id = "id-1"
				replicas     = 12
				labels = {
					"label_key1" = "label_value1",
					"label_key2" = "label_value2"
				}
				taints = [
					{
						key = "key1",
						value = "value1",
						schedule_type = "NoSchedule",
					},
				]
				version = "4.14.10"
			}`)
			Expect(terraform.Apply()).To(BeZero())

			// Check the state:
			resource := terraform.Resource("rhcs_hcp_machine_pool", "my_pool")
			Expect(resource).To(MatchJQ(".attributes.cluster", "123"))
			Expect(resource).To(MatchJQ(".attributes.id", "my-pool"))
			Expect(resource).To(MatchJQ(".attributes.name", "my-pool"))
			Expect(resource).To(MatchJQ(".attributes.aws_node_pool.instance_type", "r5.xlarge"))
			Expect(resource).To(MatchJQ(".attributes.replicas", 12.0))
			Expect(resource).To(MatchJQ(`.attributes.labels | length`, 2))
		})

		It("Can create machine pool with compute nodes when 404 (not found)", func() {
			// Prepare the server:
			server.AppendHandlers(
				CombineHandlers(
					VerifyRequest(
						http.MethodPost,
						"/api/clusters_mgmt/v1/clusters/123/node_pools",
					),
					RespondWithJSON(http.StatusCreated, `{
				  	"id": "my-pool",
				  	"aws_node_pool": {
					  	"instance_type": "r5.xlarge",
					  	"instance_profile": "bla"
				  	},
				  	"auto_repair": true,
				  	"replicas": 12,
				  	"labels": {
					    "label_key1": "label_value1",
				    	"label_key2": "label_value2"
				  	},
				  	"subnet": "id-1",
				  	"availability_zone": "us-east-1a",
			  	  	"taints": [
					  	{
							"effect": "NoSchedule",
							"key": "key1",
							"value": "value1"
					  	}
				  	],
				  	"version": {
					  	"raw_id": "4.14.10"
				  	}
				}`),
				),
			)

			// Run the apply command:
			terraform.Source(`
		resource "rhcs_hcp_machine_pool" "my_pool" {
		    cluster      = "123"
		    name         = "my-pool"
		    aws_node_pool = {
				instance_type = "r5.xlarge"
			}
			autoscaling = {
				enabled = false
			}
			subnet_id = "id-1"
		    replicas     = 12
			labels = {
				"label_key1" = "label_value1",
				"label_key2" = "label_value2"
			}
			taints = [
				{
					key = "key1",
					value = "value1",
					schedule_type = "NoSchedule",
				},
		    ]
			version = "4.14.10"
		}`)
			Expect(terraform.Apply()).To(BeZero())

			// Check the state:
			resource := terraform.Resource("rhcs_hcp_machine_pool", "my_pool")
			Expect(resource).To(MatchJQ(".attributes.cluster", "123"))
			Expect(resource).To(MatchJQ(".attributes.id", "my-pool"))
			Expect(resource).To(MatchJQ(".attributes.name", "my-pool"))
			Expect(resource).To(MatchJQ(".attributes.aws_node_pool.instance_type", "r5.xlarge"))
			Expect(resource).To(MatchJQ(".attributes.replicas", 12.0))
			Expect(resource).To(MatchJQ(`.attributes.labels | length`, 2))

			// Prepare the server for update
			server.AppendHandlers(
				CombineHandlers(
					VerifyRequest(
						http.MethodGet,
						"/api/clusters_mgmt/v1/clusters/123/node_pools/my-pool",
					),
					RespondWithJSON(http.StatusNotFound, "{}"),
				),
				CombineHandlers(
					VerifyRequest(http.MethodGet, cluster123Uri),
					RespondWithJSON(http.StatusOK, `
				{
				  "id": "123",
				  "name": "my-cluster",
				  "multi_az": true,
				  "nodes": {
					"availability_zones": [
					  "us-east-1a",
					  "us-east-1b",
					  "us-east-1c"
					]
				  },
				  "state": "ready"
				}`),
				),
				CombineHandlers(
					VerifyRequest(
						http.MethodPost,
						"/api/clusters_mgmt/v1/clusters/123/node_pools",
					),
					RespondWithJSON(http.StatusCreated, `
				{
				  	"id": "my-pool",
				  	"aws_node_pool": {
						"instance_type": "r5.xlarge",
					  	"instance_profile": "bla"
				  	},
				  	"auto_repair": true,
				  	"replicas": 12,
				  	"labels": {
					    "label_key1": "label_value1",
				    	"label_key2": "label_value2"
				  	},
				  	"subnet": "id-1",
				  	"availability_zone": "us-east-1a",
			  	  	"taints": [
					  	{
							"effect": "NoSchedule",
							"key": "key1",
							"value": "value1"
					  	}
				  	],
					"version": {
						"raw_id": "4.14.10"
					}
				}`),
				),
			)

			// Run the apply command:
			terraform.Source(`
		resource "rhcs_hcp_machine_pool" "my_pool" {
		    cluster      = "123"
		    name         = "my-pool"
		    aws_node_pool = {
				instance_type = "r5.xlarge"
			}
			autoscaling = {
				enabled = false
			}
			subnet_id = "id-1"
		    replicas     = 12
			labels = {
				"label_key1" = "label_value1",
				"label_key2" = "label_value2"
			}
			taints = [
				{
					key = "key1",
					value = "value1",
					schedule_type = "NoSchedule",
				},
		    ]
			version = "4.14.10"
		}`)
			Expect(terraform.Apply()).To(BeZero())

			// Check the state:
			resource = terraform.Resource("rhcs_hcp_machine_pool", "my_pool")
			Expect(resource).To(MatchJQ(".attributes.cluster", "123"))
			Expect(resource).To(MatchJQ(".attributes.id", "my-pool"))
			Expect(resource).To(MatchJQ(".attributes.name", "my-pool"))
			Expect(resource).To(MatchJQ(".attributes.aws_node_pool.instance_type", "r5.xlarge"))
			Expect(resource).To(MatchJQ(".attributes.replicas", 12.0))
			Expect(resource).To(MatchJQ(`.attributes.labels | length`, 2))
		})

		It("Can create machine pool with compute nodes and update labels", func() {
			// Prepare the server:
			server.AppendHandlers(
				CombineHandlers(
					VerifyRequest(
						http.MethodPost,
						"/api/clusters_mgmt/v1/clusters/123/node_pools",
					),
					RespondWithJSON(http.StatusOK, `
				{
				  "id": "my-pool",
				  "replicas": 12,
				  "availability_zone": "us-east-1a",
				  "labels": {
				    "label_key1": "label_value1",
				    "label_key2": "label_value2"
				  },
				  "subnet": "subnet-123",
				  "aws_node_pool": {
					"instance_type": "r5.xlarge",
					"instance_profile": "bla"
				  },
				  "auto_repair": true,
				  "version": {
					  "raw_id": "4.14.10"
				  }
				}`),
				),
			)

			// Run the apply command:
			terraform.Source(`
		resource "rhcs_hcp_machine_pool" "my_pool" {
		    cluster      = "123"
		    name         = "my-pool"
		    aws_node_pool = {
				instance_type = "r5.xlarge"
			}
			autoscaling = {
				enabled = false,
			}
		    replicas     = 12
			labels = {
				"label_key1" = "label_value1",
				"label_key2" = "label_value2"
			}
			version = "4.14.10"
			subnet_id = "subnet-123"
		}`)
			Expect(terraform.Apply()).To(BeZero())

			// Check the state:
			resource := terraform.Resource("rhcs_hcp_machine_pool", "my_pool")
			Expect(resource).To(MatchJQ(".attributes.cluster", "123"))
			Expect(resource).To(MatchJQ(".attributes.id", "my-pool"))
			Expect(resource).To(MatchJQ(".attributes.name", "my-pool"))
			Expect(resource).To(MatchJQ(".attributes.aws_node_pool.instance_type", "r5.xlarge"))
			Expect(resource).To(MatchJQ(".attributes.replicas", 12.0))
			Expect(resource).To(MatchJQ(`.attributes.labels | length`, 2))

			// Update - change lables
			server.AppendHandlers(
				// First get is for the Read function
				CombineHandlers(
					VerifyRequest(http.MethodGet, "/api/clusters_mgmt/v1/clusters/123/node_pools/my-pool"),
					RespondWithJSON(http.StatusOK, `
				{
				  "id": "my-pool",
				  "kind": "MachinePool",
				  "href": "/api/clusters_mgmt/v1/clusters/123/node_pools/my-pool",
	              "replicas": 12,
				  "labels": {
				    "label_key1": "label_value1",
				    "label_key2": "label_value2"
				  },
				  "aws_node_pool": {
					"instance_type": "r5.xlarge",
					"instance_profile": "bla"
				  },
				  "auto_repair": true,
				  "version": {
					  "raw_id": "4.14.10"
				  },
				  "subnet": "subnet-123"
				}`),
				),
				// Second get is for the Update function
				CombineHandlers(
					VerifyRequest(http.MethodGet, "/api/clusters_mgmt/v1/clusters/123/node_pools/my-pool"),
					RespondWithJSON(http.StatusOK, `
				{
				  "id": "my-pool",
				  "kind": "MachinePool",
				  "href": "/api/clusters_mgmt/v1/clusters/123/node_pools/my-pool",
				  "replicas": 12,
				  "labels": {
				    "label_key1": "label_value1",
				    "label_key2": "label_value2"
				  },
				  "aws_node_pool": {
					"instance_type": "r5.xlarge",
					"instance_profile": "bla"
				  },
				  "auto_repair": true,
				  "version": {
					  "raw_id": "4.14.10"
				  },
				  "subnet": "subnet-123"
				}`),
				),
				CombineHandlers(
					VerifyRequest(
						http.MethodPatch,
						"/api/clusters_mgmt/v1/clusters/123/node_pools/my-pool",
					),
					RespondWithJSON(http.StatusOK, `
				{
				  "id": "my-pool",
				  "href": "/api/clusters_mgmt/v1/clusters/123/node_pools/my-pool",
				  "kind": "MachinePool",
				  "replicas": 12,
				  "labels": {
				    "label_key3": "label_value3"
				  },
				  "aws_node_pool": {
					"instance_type": "r5.xlarge",
					"instance_profile": "bla"
				  },
				  "auto_repair": true,
				  "version": {
					  "raw_id": "4.14.10"
				  },
				  "subnet": "subnet-123"
				}`),
				),
			)

			terraform.Source(`
		resource "rhcs_hcp_machine_pool" "my_pool" {
		    cluster      = "123"
		    name         = "my-pool"
		    aws_node_pool = {
				instance_type = "r5.xlarge"
			}
		    replicas     = 12
			labels = {
				"label_key3" = "label_value3"
			}
			autoscaling = {
				enabled = false,
			}
			version = "4.14.10"
			subnet_id = "subnet-123"
		}`)
			Expect(terraform.Apply()).To(BeZero())

			// Check the state:
			resource = terraform.Resource("rhcs_hcp_machine_pool", "my_pool")
			Expect(resource).To(MatchJQ(".attributes.cluster", "123"))
			Expect(resource).To(MatchJQ(".attributes.id", "my-pool"))
			Expect(resource).To(MatchJQ(".attributes.name", "my-pool"))
			Expect(resource).To(MatchJQ(".attributes.aws_node_pool.instance_type", "r5.xlarge"))
			Expect(resource).To(MatchJQ(".attributes.replicas", 12.0))
			Expect(resource).To(MatchJQ(`.attributes.labels | length`, 1))

			// Update - delete lables
			server.AppendHandlers(
				// First get is for the Read function
				CombineHandlers(
					VerifyRequest(http.MethodGet, "/api/clusters_mgmt/v1/clusters/123/node_pools/my-pool"),
					RespondWithJSON(http.StatusOK, `
				{
				  "id": "my-pool",
				  "kind": "MachinePool",
				  "href": "/api/clusters_mgmt/v1/clusters/123/node_pools/my-pool",
	              "replicas": 12,
				  "labels": {
				    "label_key1": "label_value1",
				    "label_key2": "label_value2"
				  },
				  "aws_node_pool": {
					"instance_type": "r5.xlarge",
					"instance_profile": "bla"
				  },
				  "auto_repair": true,
				  "version": {
					  "raw_id": "4.14.10"
				  },
				  "subnet": "subnet-123"
				}`),
				),
				// Second get is for the Update function
				CombineHandlers(
					VerifyRequest(http.MethodGet, "/api/clusters_mgmt/v1/clusters/123/node_pools/my-pool"),
					RespondWithJSON(http.StatusOK, `
				{
				  "id": "my-pool",
				  "kind": "MachinePool",
				  "href": "/api/clusters_mgmt/v1/clusters/123/node_pools/my-pool",
	              "replicas": 12,
				  "labels": {
				    "label_key1": "label_value1",
				    "label_key2": "label_value2"
				  },
				  "aws_node_pool": {
					"instance_type": "r5.xlarge",
					"instance_profile": "bla"
				  },
				  "auto_repair": true,
				  "version": {
					  "raw_id": "4.14.10"
				  },
				  "subnet": "subnet-123"
				}`),
				),
				CombineHandlers(
					VerifyRequest(
						http.MethodPatch,
						"/api/clusters_mgmt/v1/clusters/123/node_pools/my-pool",
					),
					RespondWithJSON(http.StatusOK, `
				{
				  "id": "my-pool",
				  "href": "/api/clusters_mgmt/v1/clusters/123/node_pools/my-pool",
				  "kind": "MachinePool",
				  "replicas": 12,
				  "aws_node_pool": {
					"instance_type": "r5.xlarge",
					"instance_profile": "bla"
				  },
				  "auto_repair": true,
				  "version": {
					  "raw_id": "4.14.10"
				  },
				  "subnet": "subnet-123"
				}`),
				),
			)

			// Invalid deletion - labels map can't be empty
			terraform.Source(`
		resource "rhcs_hcp_machine_pool" "my_pool" {
		    cluster      = "123"
		    name         = "my-pool"
		    aws_node_pool = {
				instance_type = "r5.xlarge"
			}
		    replicas     = 12
	        labels       = {}
			autoscaling = {
				enabled = false,
			}
			version = "4.14.10"
			subnet_id = "subnet-123"
		}`)
			Expect(terraform.Apply()).ToNot(BeZero())
			// Valid deletion
			terraform.Source(`
		resource "rhcs_hcp_machine_pool" "my_pool" {
		    cluster      = "123"
		    name         = "my-pool"
		    aws_node_pool = {
				instance_type = "r5.xlarge"
			}
		    replicas     = 12
			autoscaling = {
				enabled = false,
			}
			version = "4.14.10"
			subnet_id = "subnet-123"
		}`)
			Expect(terraform.Apply()).To(BeZero())

			// Check the state:
			resource = terraform.Resource("rhcs_hcp_machine_pool", "my_pool")
			Expect(resource).To(MatchJQ(".attributes.cluster", "123"))
			Expect(resource).To(MatchJQ(".attributes.id", "my-pool"))
			Expect(resource).To(MatchJQ(".attributes.name", "my-pool"))
			Expect(resource).To(MatchJQ(".attributes.aws_node_pool.instance_type", "r5.xlarge"))
			Expect(resource).To(MatchJQ(".attributes.replicas", 12.0))
			Expect(resource).To(MatchJQ(`.attributes.labels | length`, 0))
		})

		It("Can create machine pool with compute nodes and update taints", func() {
			// Prepare the server:
			server.AppendHandlers(
				CombineHandlers(
					VerifyRequest(
						http.MethodPost,
						"/api/clusters_mgmt/v1/clusters/123/node_pools",
					),
					RespondWithJSON(http.StatusOK, `
				{
				  "id": "my-pool",
				  "replicas": 12,
				  "availability_zone": "us-east-1a",
				  "taints": [
					  {
						"effect": "NoSchedule",
						"key": "key1",
						"value": "value1"
					  }
				  ],
				  "aws_node_pool": {
					"instance_type": "r5.xlarge",
					"instance_profile": "bla"
				  },
				  "auto_repair": true,
				  "version": {
					  "raw_id": "4.14.10"
				  },
				  "subnet": "subnet-123"
				}`),
				),
			)

			// Run the apply command:
			terraform.Source(`
		  resource "rhcs_hcp_machine_pool" "my_pool" {
		    cluster      = "123"
		    name         = "my-pool"
		    aws_node_pool = {
				instance_type = "r5.xlarge"
			}
			autoscaling = {
				enabled = false,
			}
		    replicas     = 12
			taints = [
				{
					key = "key1",
					value = "value1",
					schedule_type = "NoSchedule",
				}
		    ]
			version = "4.14.10"
			subnet_id = "subnet-123"
		  }
		`)
			Expect(terraform.Apply()).To(BeZero())

			// Check the state:
			resource := terraform.Resource("rhcs_hcp_machine_pool", "my_pool")
			Expect(resource).To(MatchJQ(".attributes.cluster", "123"))
			Expect(resource).To(MatchJQ(".attributes.id", "my-pool"))
			Expect(resource).To(MatchJQ(".attributes.name", "my-pool"))
			Expect(resource).To(MatchJQ(".attributes.aws_node_pool.instance_type", "r5.xlarge"))
			Expect(resource).To(MatchJQ(".attributes.replicas", 12.0))
			Expect(resource).To(MatchJQ(`.attributes.taints | length`, 1))

			server.AppendHandlers(
				// First get is for the Read function
				CombineHandlers(
					VerifyRequest(http.MethodGet, "/api/clusters_mgmt/v1/clusters/123/node_pools/my-pool"),
					RespondWithJSON(http.StatusOK, `
				{
				  "id": "my-pool",
				  "kind": "MachinePool",
				  "href": "/api/clusters_mgmt/v1/clusters/123/node_pools/my-pool",
	              "replicas": 12,
				  "availability_zone": "us-east-1a",
				  "taints": [
					  {
						"effect": "NoSchedule",
						"key": "key1",
						"value": "value1"
					  }
				  ],
				  "aws_node_pool": {
					"instance_type": "r5.xlarge",
					"instance_profile": "bla"
				  },
				  "auto_repair": true,
				  "version": {
					  "raw_id": "4.14.10"
				  },
				  "subnet": "subnet-123"
				}`),
				),
				// Second get is for the Update function
				CombineHandlers(
					VerifyRequest(http.MethodGet, "/api/clusters_mgmt/v1/clusters/123/node_pools/my-pool"),
					RespondWithJSON(http.StatusOK, `
				{
				  "id": "my-pool",
				  "kind": "MachinePool",
				  "href": "/api/clusters_mgmt/v1/clusters/123/node_pools/my-pool",
	              "replicas": 12,
				  "availability_zone": "us-east-1a",
				  "taints": [
					  {
						"effect": "NoSchedule",
						"key": "key1",
						"value": "value1"
					  }
				  ],
				  "aws_node_pool": {
					"instance_type": "r5.xlarge",
					"instance_profile": "bla"
				  },
				  "auto_repair": true,
				  "version": {
					  "raw_id": "4.14.10"
				  },
				  "subnet": "subnet-123"
				}`),
				),
				CombineHandlers(
					VerifyRequest(
						http.MethodPatch,
						"/api/clusters_mgmt/v1/clusters/123/node_pools/my-pool",
					),
					RespondWithJSON(http.StatusOK, `
				{
				  "id": "my-pool",
				  "href": "/api/clusters_mgmt/v1/clusters/123/node_pools/my-pool",
				  "kind": "MachinePool",
				  "replicas": 12,
				  "availability_zone": "us-east-1a",
				  "taints": [
					  {
						"effect": "NoSchedule",
						"key": "key1",
						"value": "value1"
					  },
					  {
						"effect": "NoExecute",
						"key": "key2",
						"value": "value2"
					  }
				  ],
				  "aws_node_pool": {
					"instance_type": "r5.xlarge",
					"instance_profile": "bla"
				  },
				  "auto_repair": true,
				  "version": {
					  "raw_id": "4.14.10"
				  },
				  "subnet": "subnet-123"
				}`),
				),
			)

			terraform.Source(`
		  resource "rhcs_hcp_machine_pool" "my_pool" {
		    cluster      = "123"
		    name         = "my-pool"
		    aws_node_pool = {
				instance_type = "r5.xlarge"
			}
		    replicas     = 12
			taints = [
				{
					key = "key1",
					value = "value1",
					schedule_type = "NoSchedule",
				},
				{
					key = "key2",
					value = "value2",
					schedule_type = "NoExecute",
				}
		    ]
			autoscaling = {
				enabled = false,
			}
			version = "4.14.10"
			subnet_id = "subnet-123"
		  }
		`)
			Expect(terraform.Apply()).To(BeZero())

			// Check the state:
			resource = terraform.Resource("rhcs_hcp_machine_pool", "my_pool")
			Expect(resource).To(MatchJQ(".attributes.cluster", "123"))
			Expect(resource).To(MatchJQ(".attributes.id", "my-pool"))
			Expect(resource).To(MatchJQ(".attributes.name", "my-pool"))
			Expect(resource).To(MatchJQ(".attributes.aws_node_pool.instance_type", "r5.xlarge"))
			Expect(resource).To(MatchJQ(".attributes.replicas", 12.0))
			Expect(resource).To(MatchJQ(`.attributes.taints | length`, 2))
		})

		It("Can create machine pool with compute nodes and remove taints", func() {
			// Prepare the server:
			server.AppendHandlers(
				CombineHandlers(
					VerifyRequest(
						http.MethodPost,
						"/api/clusters_mgmt/v1/clusters/123/node_pools",
					),
					RespondWithJSON(http.StatusOK, `{
				  "id": "my-pool",
				  "availability_zone": "us-east-1a",
				  "replicas": 12,
				  "taints": [
					  {
						"effect": "NoSchedule",
						"key": "key1",
						"value": "value1"
					  }
				  ],
				  "aws_node_pool": {
					"instance_type": "r5.xlarge",
					"instance_profile": "bla"
				  },
				  "auto_repair": true,
				  "version": {
					  "raw_id": "4.14.10"
				  },
				  "subnet": "subnet-123"
				}`),
				),
			)

			// Run the apply command:
			terraform.Source(`
		  resource "rhcs_hcp_machine_pool" "my_pool" {
		    cluster      = "123"
		    name         = "my-pool"
		    aws_node_pool = {
				instance_type = "r5.xlarge"
			}
			autoscaling = {
				enabled = false
			}
		    replicas     = 12
			taints = [
				{
					key = "key1",
					value = "value1",
					schedule_type = "NoSchedule",
				}
		    ]
			version = "4.14.10"
			subnet_id = "subnet-123"
		  }
		`)
			Expect(terraform.Apply()).To(BeZero())

			// Check the state:
			resource := terraform.Resource("rhcs_hcp_machine_pool", "my_pool")
			Expect(resource).To(MatchJQ(".attributes.cluster", "123"))
			Expect(resource).To(MatchJQ(".attributes.id", "my-pool"))
			Expect(resource).To(MatchJQ(".attributes.name", "my-pool"))
			Expect(resource).To(MatchJQ(".attributes.aws_node_pool.instance_type", "r5.xlarge"))
			Expect(resource).To(MatchJQ(".attributes.replicas", 12.0))
			Expect(resource).To(MatchJQ(`.attributes.taints | length`, 1))

			server.AppendHandlers(
				// First get is for the Read function
				CombineHandlers(
					VerifyRequest(http.MethodGet, "/api/clusters_mgmt/v1/clusters/123/node_pools/my-pool"),
					RespondWithJSON(http.StatusOK, `
				{
				  "id": "my-pool",
				  "kind": "MachinePool",
				  "href": "/api/clusters_mgmt/v1/clusters/123/node_pools/my-pool",
	              "replicas": 12,
				  "availability_zone": "us-east-1a",
				  "taints": [
					  {
						"effect": "NoSchedule",
						"key": "key1",
						"value": "value1"
					  }
				  ],
				  "aws_node_pool": {
					"instance_type": "r5.xlarge",
					"instance_profile": "bla"
				  },
				  "auto_repair": true,
				  "version": {
					  "raw_id": "4.14.10"
				  },
				  "subnet": "subnet-123"
				}`),
				),
				// Second get is for the Update function
				CombineHandlers(
					VerifyRequest(http.MethodGet, "/api/clusters_mgmt/v1/clusters/123/node_pools/my-pool"),
					RespondWithJSON(http.StatusOK, `
				{
				  "id": "my-pool",
				  "kind": "MachinePool",
				  "href": "/api/clusters_mgmt/v1/clusters/123/node_pools/my-pool",
	              "replicas": 12,
				  "availability_zone": "us-east-1a",
				  "taints": [
					  {
						"effect": "NoSchedule",
						"key": "key1",
						"value": "value1"
					  }
				  ],
				  "aws_node_pool": {
					"instance_type": "r5.xlarge",
					"instance_profile": "bla"
				  },
				  "auto_repair": true,
				  "version": {
					  "raw_id": "4.14.10"
				  },
				  "subnet": "subnet-123"
				}`),
				),
				CombineHandlers(
					VerifyRequest(
						http.MethodPatch,
						"/api/clusters_mgmt/v1/clusters/123/node_pools/my-pool",
					),
					RespondWithJSON(http.StatusOK, `
				{
				  "id": "my-pool",
				  "href": "/api/clusters_mgmt/v1/clusters/123/node_pools/my-pool",
				  "kind": "MachinePool",
				  "replicas": 12,
				  "availability_zone": "us-east-1a",
				  "aws_node_pool": {
					"instance_type": "r5.xlarge",
					"instance_profile": "bla"
				  },
				  "auto_repair": true,
				  "version": {
					  "raw_id": "4.14.10"
				  },
				  "subnet": "subnet-123"
				}`),
				),
			)

			// invalid removal of taints
			terraform.Source(`
		  resource "rhcs_hcp_machine_pool" "my_pool" {
		    cluster      = "123"
		    name         = "my-pool"
		    aws_node_pool = {
				instance_type = "r5.xlarge"
			}
		    replicas     = 12
	        taints       = []
			autoscaling = {
				enabled = false
			}
			version = "4.14.10"
			subnet_id = "subnet-123"
		  }
		`)

			Expect(terraform.Apply()).ToNot(BeZero())

			// valid removal of taints
			terraform.Source(`
		  resource "rhcs_hcp_machine_pool" "my_pool" {
		    cluster      = "123"
		    name         = "my-pool"
		    aws_node_pool = {
				instance_type = "r5.xlarge"
			}
		    replicas     = 12
			autoscaling = {
				enabled = false
			}
			version = "4.14.10"
			subnet_id = "subnet-123"
		  }
		`)
			Expect(terraform.Apply()).To(BeZero())

			// Check the state:
			resource = terraform.Resource("rhcs_hcp_machine_pool", "my_pool")
			Expect(resource).To(MatchJQ(".attributes.cluster", "123"))
			Expect(resource).To(MatchJQ(".attributes.id", "my-pool"))
			Expect(resource).To(MatchJQ(".attributes.name", "my-pool"))
			Expect(resource).To(MatchJQ(".attributes.aws_node_pool.instance_type", "r5.xlarge"))
			Expect(resource).To(MatchJQ(".attributes.replicas", 12.0))
			Expect(resource).To(MatchJQ(`.attributes.taints | length`, 0))
		})

		It("Can create machine pool with autoscaling enabled and update to compute nodes", func() {
			// Prepare the server:
			server.AppendHandlers(
				CombineHandlers(
					VerifyRequest(
						http.MethodPost,
						"/api/clusters_mgmt/v1/clusters/123/node_pools",
					),
					RespondWithJSON(http.StatusOK, `{
				  "id": "my-pool",
				  "availability_zone": "us-east-1a",
				  "autoscaling": {
				    "max_replicas": 3,
				    "min_replicas": 0
				  },
				  "aws_node_pool": {
					"instance_type": "r5.xlarge",
					"instance_profile": "bla"
				  },
				  "auto_repair": true,
				  "version": {
					  "raw_id": "4.14.10"
				  },
				  "subnet": "subnet-123"
				}`),
				),
			)

			// Run the apply command to create the machine pool resource:
			terraform.Source(`
		resource "rhcs_hcp_machine_pool" "my_pool" {
		    cluster      = "123"
		    name         = "my-pool"
			aws_node_pool = {
				instance_type = "r5.xlarge"
			}
			autoscaling = {
				enabled = true
				min_replicas = 0
				max_replicas = 3
			}
			version = "4.14.10"
			subnet_id = "subnet-123"
		}`)
			Expect(terraform.Apply()).To(BeZero())

			// Check the state:
			resource := terraform.Resource("rhcs_hcp_machine_pool", "my_pool")
			Expect(resource).To(MatchJQ(".attributes.cluster", "123"))
			Expect(resource).To(MatchJQ(".attributes.id", "my-pool"))
			Expect(resource).To(MatchJQ(".attributes.name", "my-pool"))
			Expect(resource).To(MatchJQ(".attributes.aws_node_pool.instance_type", "r5.xlarge"))
			Expect(resource).To(MatchJQ(".attributes.autoscaling.enabled", true))
			Expect(resource).To(MatchJQ(".attributes.autoscaling.min_replicas", float64(0)))
			Expect(resource).To(MatchJQ(".attributes.autoscaling.max_replicas", float64(3)))

			server.AppendHandlers(
				// First get is for the Read function
				CombineHandlers(
					VerifyRequest(http.MethodGet, "/api/clusters_mgmt/v1/clusters/123/node_pools/my-pool"),
					RespondWithJSON(http.StatusOK, `
				{
				  "id": "my-pool",
				  "kind": "MachinePool",
				  "href": "/api/clusters_mgmt/v1/clusters/123/node_pools/my-pool",
				  "autoscaling": {
				  	"max_replicas": 3,
				  	"min_replicas": 0
				  },
				  "availability_zone": "us-east-1a",
				  "aws_node_pool": {
					"instance_type": "r5.xlarge",
					"instance_profile": "bla"
				  },
				  "auto_repair": true,
				  "version": {
					  "raw_id": "4.14.10"
				  },
				  "subnet": "subnet-123"
				}`),
				),
				// Second get is for the Update function
				CombineHandlers(
					VerifyRequest(http.MethodGet, "/api/clusters_mgmt/v1/clusters/123/node_pools/my-pool"),
					RespondWithJSON(http.StatusOK, `
				{
				  "id": "my-pool",
				  "kind": "MachinePool",
				  "href": "/api/clusters_mgmt/v1/clusters/123/node_pools/my-pool",
				  "autoscaling": {
				  	"max_replicas": 3,
				  	"min_replicas": 0
				  },
				  "availability_zone": "us-east-1a",
				  "aws_node_pool": {
					"instance_type": "r5.xlarge",
					"instance_profile": "bla"
				  },
				  "auto_repair": true,
				  "version": {
					  "raw_id": "4.14.10"
				  },
				  "subnet": "subnet-123"
				}`),
				),
				CombineHandlers(
					VerifyRequest(
						http.MethodPatch,
						"/api/clusters_mgmt/v1/clusters/123/node_pools/my-pool",
					),
					RespondWithJSON(http.StatusOK, `
				{
				  "id": "my-pool",
				  "href": "/api/clusters_mgmt/v1/clusters/123/node_pools/my-pool",
				  "kind": "MachinePool",
				  "replicas": 12,
				  "availability_zone": "us-east-1a",
				  "aws_node_pool": {
					"instance_type": "r5.xlarge",
					"instance_profile": "bla"
				  },
				  "auto_repair": true,
				  "version": {
					  "raw_id": "4.14.10"
				  },
				  "subnet": "subnet-123"
				}`),
				),
			)
			// Run the apply command to update the machine pool:
			terraform.Source(`
		  resource "rhcs_hcp_machine_pool" "my_pool" {
		    cluster      = "123"
		    name         = "my-pool"
		    aws_node_pool = {
				instance_type = "r5.xlarge"
			}
		    replicas     = 12
			autoscaling = {
				enabled = false
			}
			version = "4.14.10"
			subnet_id = "subnet-123"
		}`)
			Expect(terraform.Apply()).To(BeZero())

			// Check the state:
			resource = terraform.Resource("rhcs_hcp_machine_pool", "my_pool")
			Expect(resource).To(MatchJQ(".attributes.cluster", "123"))
			Expect(resource).To(MatchJQ(".attributes.id", "my-pool"))
			Expect(resource).To(MatchJQ(".attributes.name", "my-pool"))
			Expect(resource).To(MatchJQ(".attributes.aws_node_pool.instance_type", "r5.xlarge"))
			Expect(resource).To(MatchJQ(".attributes.replicas", float64(12)))
		})
	})

	Context("Standard workers machine pool", func() {
		BeforeEach(func() {
			server.AppendHandlers(
				CombineHandlers(
					VerifyRequest(http.MethodGet, cluster123Uri),
					RespondWithJSON(http.StatusOK, `
						{
							"id": "123",
							"name": "my-cluster",
							"multi_az": false,
							"nodes": {
								"availability_zones": [
									"us-east-1a"
								]
							},
							"state": "ready"
						}`),
				),
			)
		})

		It("cannot be created", func() {
			// Prepare the server:
			server.AppendHandlers(
				// Get is for the Read function
				CombineHandlers(
					VerifyRequest(http.MethodGet, workerNodePoolUri),
					RespondWithJSON(http.StatusNotFound, `
						{
							"kind": "Error",
							"id": "404",
							"href": "/api/clusters_mgmt/v1/errors/404",
							"code": "CLUSTERS-MGMT-404",
							"reason": "Node pool with id 'worker' not found.",
							"operation_id": "df359e0c-b1d3-4feb-9b58-50f7a20d0096"
						}`),
				),
			)
			terraform.Source(`
				resource "rhcs_hcp_machine_pool" "worker" {
					cluster      = "123"
					name         = "worker"
					aws_node_pool = {
						instance_type = "r5.xlarge"
					}
					autoscaling = {
						enabled = false
					}
					version = "4.14.10"
					replicas     = 2
					subnet_id = "subnet-123"
				}`)
			Expect(terraform.Apply()).NotTo(BeZero())
		})

		It("is automatically imported and updates applied", func() {
			// Import automatically "Create()", and update the # of replicas: 2 -> 4
			// Prepare the server:
			server.AppendHandlers(
				// Get is for the Read function
				CombineHandlers(
					VerifyRequest(http.MethodGet, workerNodePoolUri),
					RespondWithJSON(http.StatusOK, `
						{
							"id": "worker",
							"replicas": 2,
							"aws_node_pool":{
								"instance_type":"r5.xlarge",
								"instance_profile": "bla"
							},
							"version": {
								"raw_id": "4.14.10"
							},
							"subnet": "subnet-123"
						}`),
				),
				// Get is for the read during update
				CombineHandlers(
					VerifyRequest(http.MethodGet, workerNodePoolUri),
					RespondWithJSON(http.StatusOK, `
						{
							"id": "worker",
							"replicas": 2,
							"aws_node_pool":{
								"instance_type":"r5.xlarge",
								"instance_profile": "bla"
							},
							"version": {
								"raw_id": "4.14.10"
							},
							"subnet": "subnet-123"
						}`),
				),
				// Patch is for the update
				CombineHandlers(
					VerifyRequest(http.MethodPatch, workerNodePoolUri),
					RespondWithJSON(http.StatusOK, `
						{
							"id": "worker",
							"aws_node_pool":{
								"instance_type":"r5.xlarge"
							},
							"version": {
								"raw_id": "4.14.10"
							},
							"subnet": "subnet-123",
							"replicas": 4
						}`),
				),
			)
			terraform.Source(`
				resource "rhcs_hcp_machine_pool" "worker" {
					cluster      = "123"
					name         = "worker"
					aws_node_pool = {
						instance_type = "r5.xlarge"
					}
					autoscaling = {
						enabled = false
					}
					subnet_id = "subnet-123"
					version = "4.14.10"
					replicas     = 4
				}`)
			Expect(terraform.Apply()).To(BeZero())
			resource := terraform.Resource("rhcs_hcp_machine_pool", "worker")
			Expect(resource).To(MatchJQ(".attributes.cluster", "123"))
			Expect(resource).To(MatchJQ(".attributes.name", "worker"))
			Expect(resource).To(MatchJQ(".attributes.id", "worker"))
			Expect(resource).To(MatchJQ(".attributes.replicas", 4.0))
		})

		It("can update labels", func() {
			// Prepare the server:
			server.AppendHandlers(
				// Get is for the Read function
				CombineHandlers(
					VerifyRequest(http.MethodGet, workerNodePoolUri),
					RespondWithJSON(http.StatusOK, `
						{
							"id": "worker",
							"replicas": 2,
							"aws_node_pool":{
								"instance_type":"r5.xlarge",
								"instance_profile": "bla"
							},
							"version": {
								"raw_id": "4.14.10"
							},
							"subnet": "subnet-123"
						}`),
				),
				// Get is for the read during update
				CombineHandlers(
					VerifyRequest(http.MethodGet, workerNodePoolUri),
					RespondWithJSON(http.StatusOK, `
						{
							"id": "worker",
							"replicas": 2,
							"aws_node_pool":{
								"instance_type":"r5.xlarge",
								"instance_profile": "bla"
							},
							"version": {
								"raw_id": "4.14.10"
							},
							"subnet": "subnet-123"
						}`),
				),
				// Patch is for the update
				CombineHandlers(
					VerifyRequest(http.MethodPatch, workerNodePoolUri),
					RespondWithJSON(http.StatusOK, `
						{
							"id": "worker",
							"labels": {
								"label_key1": "label_value1"
							},
							"replicas": 2,
							"aws_node_pool":{
								"instance_type":"r5.xlarge",
								"instance_profile": "bla"
							},
							"version": {
								"raw_id": "4.14.10"
							},
							"subnet": "subnet-123"
						}`),
				),
			)
			terraform.Source(`
				resource "rhcs_hcp_machine_pool" "worker" {
					cluster      = "123"
					name         = "worker"
					aws_node_pool = {
						instance_type = "r5.xlarge"
					}
					autoscaling = {
						enabled = false
					}
					replicas     = 2
					labels = {
						"label_key1" = "label_value1"
					}
					subnet_id = "subnet-123"
					version = "4.14.10"
				}`)
			Expect(terraform.Apply()).To(BeZero())
			resource := terraform.Resource("rhcs_hcp_machine_pool", "worker")
			Expect(resource).To(MatchJQ(".attributes.cluster", "123"))
			Expect(resource).To(MatchJQ(".attributes.name", "worker"))
			Expect(resource).To(MatchJQ(".attributes.id", "worker"))
			Expect(resource).To(MatchJQ(`.attributes.labels | length`, 1))
		})
	})

	Context("Machine pool creation for non exist cluster", func() {
		It("Fail to create machine pool if cluster is not exist", func() {
			// Prepare the server:
			server.AppendHandlers(
				// Get is for the Read function
				CombineHandlers(
					VerifyRequest(http.MethodGet, cluster123Uri),
					RespondWithJSON(http.StatusNotFound, `{}`),
				),
			)

			// Run the apply command:
			terraform.Source(`
			  resource "rhcs_hcp_machine_pool" "my_pool" {
				cluster      = "123"
				name         = "my-pool"
				aws_node_pool = {
					instance_type = "r5.xlarge"
				}
				autoscaling = {
					enabled = false
				}
				replicas     = 4
				subnet_id = "not-in-vpc-of-cluster"
				version = "4.14.10"
			  }
			`)
			Expect(terraform.Apply()).NotTo(BeZero())
		})
	})

	Context("Import", func() {
		It("Can import a machine pool", func() {
			// Prepare the server:
			server.AppendHandlers(
				// Get is for the Read function
				CombineHandlers(
					VerifyRequest(http.MethodGet, "/api/clusters_mgmt/v1/clusters/123/node_pools/my-pool"),
					RespondWithJSON(http.StatusOK, `
						{
						  "id": "my-pool",
						  "kind": "MachinePool",
						  "href": "/api/clusters_mgmt/v1/clusters/123/node_pools/my-pool",
						  "replicas": 12,
						  "labels": {
							"label_key1": "label_value1",
							"label_key2": "label_value2"
						  },
						  "aws_node_pool": {
							"instance_type": "r5.xlarge",
							"instance_profile": "bla"
						  },
						  "auto_repair": true,
						  "version": {
							  "raw_id": "4.14.10"
						  }
						}`),
				),
			)

			// Run the import command:
			terraform.Source(`resource "rhcs_hcp_machine_pool" "my_pool" {}`)
			Expect(terraform.Import("rhcs_hcp_machine_pool.my_pool", "123,my-pool")).To(BeZero())
			resource := terraform.Resource("rhcs_hcp_machine_pool", "my_pool")
			Expect(resource).To(MatchJQ(".attributes.cluster", "123"))
			Expect(resource).To(MatchJQ(".attributes.name", "my-pool"))
			Expect(resource).To(MatchJQ(".attributes.id", "my-pool"))
		})
	})

	Context("Machine pool delete", func() {
		clusterId := "123"

		prepareClusterRead := func(clusterId string) {
			server.AppendHandlers(
				CombineHandlers(
					VerifyRequest(http.MethodGet, "/api/clusters_mgmt/v1/clusters/"+clusterId),
					RespondWithJSONTemplate(http.StatusOK, `{
					  "id": "{{.ClusterId}}",
					  "name": "my-cluster",
					  "multi_az": true,
					  "nodes": {
						"availability_zones": [
						  "us-east-1a",
						  "us-east-1b",
						  "us-east-1c"
						]
					  },
					  "state": "ready"
					}`, "ClusterId", clusterId),
				),
			)
		}

		preparePoolRead := func(clusterId string, poolId string) {
			server.AppendHandlers(
				CombineHandlers(
					VerifyRequest(http.MethodGet, "/api/clusters_mgmt/v1/clusters/"+clusterId+"/node_pools/"+poolId),
					RespondWithJSONTemplate(http.StatusOK, `
				{
					"id": "{{.PoolId}}",
					"kind": "NodePool",
					"href": "/api/clusters_mgmt/v1/clusters/{{.ClusterId}}/node_pools/{{.PoolId}}",
					"replicas": 3,
					"aws_node_pool":{
						"instance_type":"r5.xlarge",
						"instance_profile": "bla"
					},
					"version": {
						"raw_id": "4.14.10"
					},
					"subnet": "subnet-123"
				}`, "PoolId", poolId, "ClusterId", clusterId),
				),
			)
		}

		createPool := func(clusterId string, poolId string) {
			prepareClusterRead(clusterId)
			server.AppendHandlers(
				CombineHandlers(
					VerifyRequest(
						http.MethodPost,
						"/api/clusters_mgmt/v1/clusters/"+clusterId+"/node_pools",
					),
					RespondWithJSONTemplate(http.StatusOK, `{
						  "id": "{{.PoolId}}",
						  "name": "{{.PoolId}}",
						  "aws_node_pool":{
							 "instance_type":"r5.xlarge",
							 "instance_profile": "bla"
						  },
						  "replicas": 3,
						  "availability_zone": "us-east-1a",
						"version": {
							"raw_id": "4.14.10"
						},
						"subnet": "subnet-123"
					}`, "PoolId", poolId),
				),
			)

			terraform.Source(EvaluateTemplate(`
			resource "rhcs_hcp_machine_pool" "{{.PoolId}}" {
				  cluster      = "{{.ClusterId}}"
				  name         = "{{.PoolId}}"
				  aws_node_pool = {
					instance_type = "r5.xlarge"
				}
				replicas     = 3
				subnet_id = "subnet-123"
				autoscaling = {
					enabled = false
				}
				version = "4.14.10"
			}`, "PoolId", poolId, "ClusterId", clusterId))

			// Run the apply command:
			Expect(terraform.Apply()).To(BeZero())
			resource := terraform.Resource("rhcs_hcp_machine_pool", poolId)
			Expect(resource).To(MatchJQ(".attributes.cluster", clusterId))
			Expect(resource).To(MatchJQ(".attributes.id", poolId))
			Expect(resource).To(MatchJQ(".attributes.name", poolId))
		}

		BeforeEach(func() {
			createPool(clusterId, "pool1")
		})

		It("can delete a machine pool", func() {
			// Prepare for refresh (Read) of the pools prior to changes
			preparePoolRead(clusterId, "pool1")
			// Prepare for the delete of pool1
			server.AppendHandlers(
				CombineHandlers(
					VerifyRequest(http.MethodDelete, "/api/clusters_mgmt/v1/clusters/"+clusterId+"/node_pools/pool1"),
					RespondWithJSON(http.StatusOK, `{}`),
				),
			)

			// Re-apply w/ empty source so that pool1 is deleted
			terraform.Source("")
			Expect(terraform.Apply()).To(BeZero())
		})
		It("will return an error if delete fails and not the last pool", func() {
			// Prepare for refresh (Read) of the pools prior to changes
			preparePoolRead(clusterId, "pool1")
			// Prepare for the delete of pool1
			server.AppendHandlers(
				CombineHandlers( // Fail the delete
					VerifyRequest(http.MethodDelete, "/api/clusters_mgmt/v1/clusters/"+clusterId+"/node_pools/pool1"),
					RespondWithJSON(http.StatusBadRequest, `{}`), // XXX Fix description
				),
				CombineHandlers( // List returns more than 1 pool
					VerifyRequest(http.MethodGet, "/api/clusters_mgmt/v1/clusters/"+clusterId+"/node_pools"),
					RespondWithJSONTemplate(http.StatusOK, `{
						"kind": "NodePoolList",
						"href": "/api/clusters_mgmt/v1/clusters/{{.ClusterId}}/node_pools",
						"page": 1,
						"size": 2,
						"total": 2,
						"items": [
						  {
							"kind": "NodePool",
							"href": "/api/clusters_mgmt/v1/clusters/{{.ClusterId}}/node_pools/worker",
							"id": "worker",
							"replicas": 2,
							"availability_zone": "us-east-1a",
							"aws_node_pool":{
							   "instance_type":"r5.xlarge",
							   "instance_profile": "bla"
							},
							"version": {
								"raw_id": "4.14.10"
							},
							"subnet": "subnet-123"
						  },
						  {
							"kind": "NodePool",
							"href": "/api/clusters_mgmt/v1/clusters/{{.ClusterId}}/node_pools/pool1",
							"id": "pool1",
							"replicas": 2,
							"availability_zone": "us-east-1a",
							"aws_node_pool":{
							   "instance_type":"r5.xlarge",
							   "instance_profile": "bla"
							},
							"version": {
								"raw_id": "4.14.10"
							},
							"subnet": "subnet-123"
						  }
						]
					  }`),
				),
			)

			// Re-apply w/ empty source so that pool1 is (attempted) deleted
			terraform.Source("")
			Expect(terraform.Apply()).NotTo(BeZero())
		})
		It("will ignore the error if delete fails and is the last pool", func() {
			// Prepare for refresh (Read) of the pools prior to changes
			preparePoolRead(clusterId, "pool1")
			// Prepare for the delete of pool1
			server.AppendHandlers(
				CombineHandlers( // Fail the delete
					VerifyRequest(http.MethodDelete, "/api/clusters_mgmt/v1/clusters/"+clusterId+"/node_pools/pool1"),
					RespondWithJSON(http.StatusBadRequest, `{}`), // XXX Fix description
				),
				CombineHandlers( // List returns only 1 pool
					VerifyRequest(http.MethodGet, "/api/clusters_mgmt/v1/clusters/"+clusterId+"/node_pools"),
					RespondWithJSONTemplate(http.StatusOK, `{
						"kind": "NodePoolList",
						"href": "/api/clusters_mgmt/v1/clusters/{{.ClusterId}}/node_pools",
						"page": 1,
						"size": 1,
						"total": 1,
						"items": [
						  {
							"kind": "NodePool",
							"href": "/api/clusters_mgmt/v1/clusters/{{.ClusterId}}/node_pools/pool1",
							"id": "pool1",
							"replicas": 2,
							"availability_zone": "us-east-1a",
							"aws_node_pool":{
							   "instance_type":"r5.xlarge",
							   "instance_profile": "bla"
							},
							"version": {
								"raw_id": "4.14.10"
							},
							"subnet": "subnet-123"
						  }
						]
					  }`),
				),
			)

			// Re-apply w/ empty source so that pool1 is (attempted) deleted
			terraform.Source("")
			// Last pool, we ignore the error, so this succeeds
			Expect(terraform.Apply()).To(BeZero())
		})
	})

	Context("Upgrade", func() {
		clusterId := "123"
		poolId := "pool1"
		const emptyNodePoolUpgradePolicies = `
		{
			"page": 1,
			"size": 0,
			"total": 0,
			"items": []
		}`
		v4141Spec, err := cmv1.NewVersion().ChannelGroup("stable").
			Enabled(true).
			ROSAEnabled(true).
			HostedControlPlaneEnabled(true).
			ID("openshift-v4.14.1").
			RawID("v4.14.1").Build()
		b := new(strings.Builder)
		err = cmv1.MarshalVersion(v4141Spec, b)
		Expect(err).ToNot(HaveOccurred())
		v4141Info := b.String()
		prepareClusterRead := func(clusterId string) {
			server.AppendHandlers(
				CombineHandlers(
					VerifyRequest(http.MethodGet, "/api/clusters_mgmt/v1/clusters/"+clusterId),
					RespondWithJSONTemplate(http.StatusOK, `
					{
						"id": "{{.ClusterId}}",
						"name": "my-cluster",
						"multi_az": true,
						"nodes": {
							"availability_zones": [
								"us-east-1a",
								"us-east-1b",
								"us-east-1c"
							]
						},
						"state": "ready",
						"version": {
							"channel_group": "stable",
							"id": "openshitf-v4.14.0",
							"raw_id": "4.14.0",
							"enabled": true,
							"rosa_enabled": true,
							"hosted_control_plane_enabled": true,
							"available_upgrades": ["4.14.1"]
						}
					}`, "ClusterId", clusterId),
				),
			)
		}

		preparePoolRead := func(clusterId string, poolId string) {
			server.AppendHandlers(
				CombineHandlers(
					VerifyRequest(http.MethodGet, "/api/clusters_mgmt/v1/clusters/"+clusterId+"/node_pools/"+poolId),
					RespondWithJSONTemplate(http.StatusOK, `
					{
						"id": "{{.PoolId}}",
						"kind": "NodePool",
						"href": "/api/clusters_mgmt/v1/clusters/{{.ClusterId}}/node_pools/{{.PoolId}}",
						"replicas": 3,
						"aws_node_pool":{
							"instance_type":"r5.xlarge",
							"instance_profile": "bla"
						},
						"version": {
							"channel_group": "stable",
							"id": "openshift-v4.14.0",
							"raw_id": "4.14.0",
							"enabled": true,
							"rosa_enabled": true,
							"hosted_control_plane_enabled": true,
							"available_upgrades": ["4.14.1"]
						},
						"subnet": "subnet-123"
					}`, "PoolId", poolId, "ClusterId", clusterId),
				),
			)
		}

		createPool := func(clusterId string, poolId string) {
			prepareClusterRead(clusterId)
			server.AppendHandlers(
				CombineHandlers(
					VerifyRequest(
						http.MethodPost,
						"/api/clusters_mgmt/v1/clusters/"+clusterId+"/node_pools",
					),
					RespondWithJSONTemplate(http.StatusOK, `{
						"id": "{{.PoolId}}",
						"name": "{{.PoolId}}",
						"aws_node_pool":{
							"instance_type":"r5.xlarge",
							"instance_profile": "bla"
						},
						"replicas": 3,
						"availability_zone": "us-east-1a",
						"version": {
							"raw_id": "4.14.0"
						},
						"subnet": "subnet-123"
					}`, "PoolId", poolId),
				),
			)

			terraform.Source(EvaluateTemplate(`
			resource "rhcs_hcp_machine_pool" "{{.PoolId}}" {
				  cluster      = "{{.ClusterId}}"
				  name         = "{{.PoolId}}"
				  aws_node_pool = {
					instance_type = "r5.xlarge"
				}
				replicas     = 3
				subnet_id = "subnet-123"
				autoscaling = {
					enabled = false
				}
				version = "4.14.0"
			}`, "PoolId", poolId, "ClusterId", clusterId))

			// Run the apply command:
			Expect(terraform.Apply()).To(BeZero())
			resource := terraform.Resource("rhcs_hcp_machine_pool", poolId)
			Expect(resource).To(MatchJQ(".attributes.cluster", clusterId))
			Expect(resource).To(MatchJQ(".attributes.id", poolId))
			Expect(resource).To(MatchJQ(".attributes.name", poolId))
		}

		BeforeEach(func() {
			createPool(clusterId, "pool1")
		})

		It("Upgrades Machine Pool", func() {
			preparePoolRead(clusterId, poolId)
			preparePoolRead(clusterId, poolId)
			prepareClusterRead(clusterId)
			preparePoolRead(clusterId, poolId)
			server.AppendHandlers(
				CombineHandlers(
					VerifyRequest(http.MethodGet, "/api/clusters_mgmt/v1/versions/openshift-v4.14.1"),
					RespondWithJSON(http.StatusOK, v4141Info),
				),
				// Look for existing upgrade policies
				CombineHandlers(
					VerifyRequest(http.MethodGet, cluster123Route+"/node_pools/pool1/upgrade_policies"),
					RespondWithJSON(http.StatusOK, emptyNodePoolUpgradePolicies),
				),
				// Look for gate agreements by posting an upgrade policy w/ dryRun
				CombineHandlers(
					VerifyRequest(http.MethodPost, cluster123Route+"/node_pools/pool1/upgrade_policies", "dryRun=true"),
					VerifyJQ(".version", "4.14.1"),
					RespondWithJSON(http.StatusBadRequest, `{
						"kind": "Error",
						"id": "400",
						"href": "/api/clusters_mgmt/v1/errors/400",
						"code": "CLUSTERS-MGMT-400",
						"reason": "There are missing version gate agreements for this cluster. See details.",
						"details": [
						{
							"kind": "VersionGate",
							"id": "999",
							"href": "/api/clusters_mgmt/v1/version_gates/596326fb-d1ea-11ed-9f29-0a580a8312f9",
							"version_raw_id_prefix": "4.14",
							"label": "api.openshift.com/gate-sts",
							"value": "4.14",
							"warning_message": "STS roles must be updated blah blah blah",
							"description": "OpenShift STS clusters include new required cloud provider permissions in OpenShift 4.YY.",
							"documentation_url": "https://access.redhat.com/solutions/0000000",
							"sts_only": true,
							"creation_timestamp": "2023-04-03T06:39:57.057613Z"
						}
						],
						"operation_id": "8f2d2946-c4ef-4c2f-877b-c19eb17dc918"
					}`),
				),
				// Send acks for all gate agreements
				CombineHandlers(
					VerifyRequest(http.MethodPost, cluster123Route+"/gate_agreements"),
					VerifyJQ(".version_gate.id", "999"),
					RespondWithJSON(http.StatusCreated, `{
						"kind": "VersionGateAgreement",
						"id": "888",
						"href": "/api/clusters_mgmt/v1/clusters/24g9q8jhdhv66fi41jfiuup5lsvu61fi/gate_agreements/d2e8d371-1033-11ee-9f05-0a580a820bdb",
						"version_gate": {
						"kind": "VersionGate",
						"id": "999",
						"href": "/api/clusters_mgmt/v1/version_gates/596326fb-d1ea-11ed-9f29-0a580a8312f9",
						"version_raw_id_prefix": "4.14",
						"label": "api.openshift.com/gate-sts",
						"value": "4.14",
						"warning_message": "STS blah blah blah",
						"description": "OpenShift STS clusters include new required cloud provider permissions in OpenShift 4.YY.",
						"documentation_url": "https://access.redhat.com/solutions/0000000",
						"sts_only": true,
						"creation_timestamp": "2023-04-03T06:39:57.057613Z"
						},
						"creation_timestamp": "2023-06-21T13:02:06.291443Z"
					}`),
				),
				// Create an upgrade policy
				CombineHandlers(
					VerifyRequest(http.MethodPost, cluster123Route+"/node_pools/pool1/upgrade_policies"),
					VerifyJQ(".version", "4.14.1"),
					RespondWithJSON(http.StatusCreated, `
					{
						"id": "123",
						"schedule_type": "manual",
						"upgrade_type": "OSD",
						"version": "4.14.1",
						"next_run": "2023-06-09T20:59:00Z",
						"cluster_id": "123",
						"enable_minor_version_upgrades": true
					}`),
				),
				// Patch the cluster (w/ no changes)
				CombineHandlers(
					VerifyRequest(http.MethodPatch, cluster123Route+"/node_pools/pool1"),
					RespondWithJSON(http.StatusOK, `
					{
						"id": "pool1",
						"replicas": 3,
						"subnet": "subnet-123"
					}`),
				),
			)

			terraform.Source(EvaluateTemplate(`
			resource "rhcs_hcp_machine_pool" "{{.PoolId}}" {
				cluster      = "{{.ClusterId}}"
				name         = "{{.PoolId}}"
				aws_node_pool = {
					instance_type = "r5.xlarge"
				}
				replicas     = 3
				subnet_id = "subnet-123"
				autoscaling = {
					enabled = false
				}
				version = "4.14.1"
			}`, "PoolId", poolId, "ClusterId", clusterId))

			// Run the apply command:
			Expect(terraform.Apply()).To(BeZero())
		})

		It("Does nothing if upgrade is in progress to a different version than the desired", func() {
			preparePoolRead(clusterId, poolId)
			preparePoolRead(clusterId, poolId)
			prepareClusterRead(clusterId)
			preparePoolRead(clusterId, poolId)
			server.AppendHandlers(
				CombineHandlers(
					VerifyRequest(http.MethodGet, "/api/clusters_mgmt/v1/versions/openshift-v4.14.1"),
					RespondWithJSON(http.StatusOK, v4141Info),
				),
				// Look for existing upgrade policies
				CombineHandlers(
					VerifyRequest(http.MethodGet, cluster123Route+"/node_pools/pool1/upgrade_policies"),
					RespondWithJSON(http.StatusOK, `{
						"page": 1,
						"size": 1,
						"total": 1,
						"items": [
							{
								"id": "456",
								"schedule_type": "manual",
								"upgrade_type": "NodePool",
								"version": "4.14.0",
								"next_run": "2023-06-09T20:59:00Z",
								"cluster_id": "123",
								"enable_minor_version_upgrades": true
							}
						]
					}`),
				),
				// Check it's state
				CombineHandlers(
					VerifyRequest(http.MethodGet, cluster123Route+"/node_pools/pool1/upgrade_policies/456"),
					RespondWithJSON(http.StatusOK, `
					{
						"id": "456",
						"state": {
							"description": "Upgrade in progress",
							"value": "started"
						}
					}`),
				),
			)

			terraform.Source(EvaluateTemplate(`
			resource "rhcs_hcp_machine_pool" "{{.PoolId}}" {
				cluster      = "{{.ClusterId}}"
				name         = "{{.PoolId}}"
				aws_node_pool = {
					instance_type = "r5.xlarge"
				}
				replicas     = 3
				subnet_id = "subnet-123"
				autoscaling = {
					enabled = false
				}
				version = "4.14.1"
			}`, "PoolId", poolId, "ClusterId", clusterId))

			// Will fail due to upgrade in progress
			Expect(terraform.Apply()).NotTo(BeZero())
		})

		It("Cancels and upgrade for the wrong version & schedules new", func() {
			preparePoolRead(clusterId, poolId)
			preparePoolRead(clusterId, poolId)
			prepareClusterRead(clusterId)
			preparePoolRead(clusterId, poolId)
			server.AppendHandlers(
				CombineHandlers(
					VerifyRequest(http.MethodGet, "/api/clusters_mgmt/v1/versions/openshift-v4.14.1"),
					RespondWithJSON(http.StatusOK, v4141Info),
				),
				// Look for existing upgrade policies
				CombineHandlers(
					VerifyRequest(http.MethodGet, cluster123Route+"/node_pools/pool1/upgrade_policies"),
					RespondWithJSON(http.StatusOK, `{
						"page": 1,
						"size": 1,
						"total": 1,
						"items": [
							{
								"id": "456",
								"node_pool_id": "pool1",
								"cluster_id": "123",
								"schedule_type": "manual",
								"upgrade_type": "NodePool",
								"version": "4.14.0",
								"next_run": "2023-06-09T20:59:00Z",
								"cluster_id": "123",
								"enable_minor_version_upgrades": true
							}
						]
					}`),
				),
				// Check it's state
				CombineHandlers(
					VerifyRequest(http.MethodGet, cluster123Route+"/node_pools/pool1/upgrade_policies/456"),
					RespondWithJSON(http.StatusOK, `
					{
						"id": "456",
						"cluster_id": "123",
						"state": {
							"description": "Upgrade in progress",
							"value": "scheduled"
						}
					}`),
				),
				// Delete existing upgrade policy
				CombineHandlers(
					VerifyRequest(http.MethodDelete, cluster123Route+"/node_pools/pool1/upgrade_policies/456"),
					RespondWithJSON(http.StatusOK, "{}"),
				),
				// Look for gate agreements by posting an upgrade policy w/ dryRun (no gates necessary)
				CombineHandlers(
					VerifyRequest(http.MethodPost, cluster123Route+"/node_pools/pool1/upgrade_policies", "dryRun=true"),
					VerifyJQ(".version", "4.14.1"),
					RespondWithJSON(http.StatusNoContent, ""),
				),
				// Create an upgrade policy
				CombineHandlers(
					VerifyRequest(http.MethodPost, cluster123Route+"/node_pools/pool1/upgrade_policies"),
					VerifyJQ(".version", "4.14.1"),
					RespondWithJSON(http.StatusCreated, `
					{
						"id": "123",
						"schedule_type": "manual",
						"upgrade_type": "OSD",
						"version": "4.14.1",
						"next_run": "2023-06-09T20:59:00Z",
						"cluster_id": "123",
						"enable_minor_version_upgrades": true
					}`),
				),
				// Patch the cluster (w/ no changes)
				CombineHandlers(
					VerifyRequest(http.MethodPatch, cluster123Route+"/node_pools/pool1"),
					RespondWithJSON(http.StatusOK, `
					{
						"id": "pool1",
						"replicas": 3,
						"subnet": "subnet-123"
					}`),
				),
			)

			terraform.Source(EvaluateTemplate(`
			resource "rhcs_hcp_machine_pool" "{{.PoolId}}" {
				cluster      = "{{.ClusterId}}"
				name         = "{{.PoolId}}"
				aws_node_pool = {
					instance_type = "r5.xlarge"
				}
				replicas     = 3
				subnet_id = "subnet-123"
				autoscaling = {
					enabled = false
				}
				version = "4.14.1"
			}`, "PoolId", poolId, "ClusterId", clusterId))

			Expect(terraform.Apply()).To(BeZero())
		})

		It("Cancels upgrade if version=current_version", func() {
			preparePoolRead(clusterId, poolId)
			preparePoolRead(clusterId, poolId)
			prepareClusterRead(clusterId)
			preparePoolRead(clusterId, poolId)
			server.AppendHandlers(
				CombineHandlers(
					VerifyRequest(http.MethodGet, "/api/clusters_mgmt/v1/versions/openshift-v4.14.1"),
					RespondWithJSON(http.StatusOK, v4141Info),
				),
				// Look for existing upgrade policies
				CombineHandlers(
					VerifyRequest(http.MethodGet, cluster123Route+"/node_pools/pool1/upgrade_policies"),
					RespondWithJSON(http.StatusOK, `{
						"page": 1,
						"size": 1,
						"total": 1,
						"items": [
							{
								"id": "456",
								"node_pool_id": "pool1",
								"cluster_id": "123",
								"schedule_type": "manual",
								"upgrade_type": "NodePool",
								"version": "4.14.0",
								"next_run": "2023-06-09T20:59:00Z",
								"cluster_id": "123",
								"enable_minor_version_upgrades": true
							}
						]
					}`),
				),
				// Check it's state
				CombineHandlers(
					VerifyRequest(http.MethodGet, cluster123Route+"/node_pools/pool1/upgrade_policies/456"),
					RespondWithJSON(http.StatusOK, `
					{
						"id": "456",
						"cluster_id": "123",
						"state": {
							"description": "Upgrade in progress",
							"value": "scheduled"
						}
					}`),
				),
				// Delete existing upgrade policy
				CombineHandlers(
					VerifyRequest(http.MethodDelete, cluster123Route+"/node_pools/pool1/upgrade_policies/456"),
					RespondWithJSON(http.StatusOK, "{}"),
				),
				// Patch the cluster (w/ no changes)
				CombineHandlers(
					VerifyRequest(http.MethodPatch, cluster123Route+"/node_pools/pool1"),
					RespondWithJSON(http.StatusOK, `
					{
						"id": "pool1",
						"replicas": 3,
						"subnet": "subnet-123"
					}`),
				),
			)

			terraform.Source(EvaluateTemplate(`
			resource "rhcs_hcp_machine_pool" "{{.PoolId}}" {
				cluster      = "{{.ClusterId}}"
				name         = "{{.PoolId}}"
				aws_node_pool = {
					instance_type = "r5.xlarge"
				}
				replicas     = 3
				subnet_id = "subnet-123"
				autoscaling = {
					enabled = false
				}
				version = "4.14.0"
			}`, "PoolId", poolId, "ClusterId", clusterId))

			Expect(terraform.Apply()).To(BeZero())
		})

		It("is an error to request a version older than current", func() {
			server.AppendHandlers(
				CombineHandlers(
					VerifyRequest(http.MethodGet, "/api/clusters_mgmt/v1/clusters/"+clusterId+"/node_pools/"+poolId),
					RespondWithJSONTemplate(http.StatusOK, `
					{
						"id": "{{.PoolId}}",
						"kind": "NodePool",
						"href": "/api/clusters_mgmt/v1/clusters/{{.ClusterId}}/node_pools/{{.PoolId}}",
						"replicas": 3,
						"aws_node_pool":{
							"instance_type":"r5.xlarge",
							"instance_profile": "bla"
						},
						"version": {
							"channel_group": "stable",
							"id": "openshift-v4.14.2",
							"raw_id": "4.14.2",
							"enabled": true,
							"rosa_enabled": true,
							"hosted_control_plane_enabled": true,
							"available_upgrades": ["4.14.3"]
						},
						"subnet": "subnet-123"
					}`, "PoolId", poolId, "ClusterId", clusterId),
				),
			)
			server.AppendHandlers(
				CombineHandlers(
					VerifyRequest(http.MethodGet, "/api/clusters_mgmt/v1/clusters/"+clusterId+"/node_pools/"+poolId),
					RespondWithJSONTemplate(http.StatusOK, `
					{
						"id": "{{.PoolId}}",
						"kind": "NodePool",
						"href": "/api/clusters_mgmt/v1/clusters/{{.ClusterId}}/node_pools/{{.PoolId}}",
						"replicas": 3,
						"aws_node_pool":{
							"instance_type":"r5.xlarge",
							"instance_profile": "bla"
						},
						"version": {
							"channel_group": "stable",
							"id": "openshift-v4.14.2",
							"raw_id": "4.14.2",
							"enabled": true,
							"rosa_enabled": true,
							"hosted_control_plane_enabled": true,
							"available_upgrades": ["4.14.3"]
						},
						"subnet": "subnet-123"
					}`, "PoolId", poolId, "ClusterId", clusterId),
				),
			)

			terraform.Source(EvaluateTemplate(`
			resource "rhcs_hcp_machine_pool" "{{.PoolId}}" {
				cluster      = "{{.ClusterId}}"
				name         = "{{.PoolId}}"
				aws_node_pool = {
					instance_type = "r5.xlarge"
				}
				replicas     = 3
				subnet_id = "subnet-123"
				autoscaling = {
					enabled = false
				}
				version = "4.14.1"
			}`, "PoolId", poolId, "ClusterId", clusterId))

			Expect(terraform.Apply()).NotTo(BeZero())
		})

		It("older than current is allowed as long as not changed", func() {
			server.AppendHandlers(
				CombineHandlers(
					VerifyRequest(http.MethodGet, "/api/clusters_mgmt/v1/clusters/"+clusterId+"/node_pools/"+poolId),
					RespondWithJSONTemplate(http.StatusOK, `
					{
						"id": "{{.PoolId}}",
						"kind": "NodePool",
						"href": "/api/clusters_mgmt/v1/clusters/{{.ClusterId}}/node_pools/{{.PoolId}}",
						"replicas": 3,
						"aws_node_pool":{
							"instance_type":"r5.xlarge",
							"instance_profile": "bla"
						},
						"version": {
							"channel_group": "stable",
							"id": "openshift-v4.14.1",
							"raw_id": "4.14.1",
							"enabled": true,
							"rosa_enabled": true,
							"hosted_control_plane_enabled": true
						},
						"subnet": "subnet-123"
					}`, "PoolId", poolId, "ClusterId", clusterId),
				),
			)
			server.AppendHandlers(
				CombineHandlers(
					VerifyRequest(http.MethodGet, "/api/clusters_mgmt/v1/clusters/"+clusterId+"/node_pools/"+poolId),
					RespondWithJSONTemplate(http.StatusOK, `
					{
						"id": "{{.PoolId}}",
						"kind": "NodePool",
						"href": "/api/clusters_mgmt/v1/clusters/{{.ClusterId}}/node_pools/{{.PoolId}}",
						"replicas": 3,
						"aws_node_pool":{
							"instance_type":"r5.xlarge",
							"instance_profile": "bla"
						},
						"version": {
							"channel_group": "stable",
							"id": "openshift-v4.14.1",
							"raw_id": "4.14.1",
							"enabled": true,
							"rosa_enabled": true,
							"hosted_control_plane_enabled": true
						},
						"subnet": "subnet-123"
					}`, "PoolId", poolId, "ClusterId", clusterId),
				),
			)

			terraform.Source(EvaluateTemplate(`
			resource "rhcs_hcp_machine_pool" "{{.PoolId}}" {
				cluster      = "{{.ClusterId}}"
				name         = "{{.PoolId}}"
				aws_node_pool = {
					instance_type = "r5.xlarge"
				}
				replicas     = 3
				subnet_id = "subnet-123"
				autoscaling = {
					enabled = false
				}
				version = "4.14.0"
			}`, "PoolId", poolId, "ClusterId", clusterId))

			Expect(terraform.Apply()).To(BeZero())
		})

		Context("Un-acked gates", func() {
			BeforeEach(func() {
				preparePoolRead(clusterId, poolId)
				preparePoolRead(clusterId, poolId)
				prepareClusterRead(clusterId)
				preparePoolRead(clusterId, poolId)
				server.AppendHandlers(
					CombineHandlers(
						VerifyRequest(http.MethodGet, "/api/clusters_mgmt/v1/versions/openshift-v4.14.1"),
						RespondWithJSON(http.StatusOK, v4141Info),
					),
					// Look for existing upgrade policies
					CombineHandlers(
						VerifyRequest(http.MethodGet, cluster123Route+"/node_pools/pool1/upgrade_policies"),
						RespondWithJSON(http.StatusOK, emptyNodePoolUpgradePolicies),
					),
					// Look for gate agreements by posting an upgrade policy w/ dryRun
					CombineHandlers(
						VerifyRequest(http.MethodPost, cluster123Route+"/node_pools/pool1/upgrade_policies", "dryRun=true"),
						VerifyJQ(".version", "4.14.1"),
						RespondWithJSON(http.StatusBadRequest, `{
							"kind": "Error",
							"id": "400",
							"href": "/api/clusters_mgmt/v1/errors/400",
							"code": "CLUSTERS-MGMT-400",
							"reason": "There are missing version gate agreements for this cluster. See details.",
							"details": [
							{
								"id": "999",
								"version_raw_id_prefix": "4.14",
								"label": "api.openshift.com/ackme",
								"value": "4.14",
								"warning_message": "user gotta ack",
								"description": "deprecations... blah blah blah",
								"documentation_url": "https://access.redhat.com/solutions/0000000",
								"sts_only": false,
								"creation_timestamp": "2023-04-03T06:39:57.057613Z"
							}
							],
							"operation_id": "8f2d2946-c4ef-4c2f-877b-c19eb17dc918"
						}`),
					),
				)
			})
			It("Fails upgrade for un-acked gates", func() {
				terraform.Source(EvaluateTemplate(`
				resource "rhcs_hcp_machine_pool" "{{.PoolId}}" {
					cluster      = "{{.ClusterId}}"
					name         = "{{.PoolId}}"
					aws_node_pool = {
						instance_type = "r5.xlarge"
					}
					replicas     = 3
					subnet_id = "subnet-123"
					autoscaling = {
						enabled = false
					}
					version = "4.14.1"
				}`, "PoolId", poolId, "ClusterId", clusterId))

				// Run the apply command:
				Expect(terraform.Apply()).NotTo(BeZero())
			})
			It("Fails upgrade if wrong version is acked", func() {
				terraform.Source(EvaluateTemplate(`
				resource "rhcs_hcp_machine_pool" "{{.PoolId}}" {
					cluster      = "{{.ClusterId}}"
					name         = "{{.PoolId}}"
					aws_node_pool = {
						instance_type = "r5.xlarge"
					}
					replicas     = 3
					subnet_id = "subnet-123"
					autoscaling = {
						enabled = false
					}
					version = "4.14.1"
					upgrade_acknowledgements_for = "1.1"
				}`, "PoolId", poolId, "ClusterId", clusterId))

				// Run the apply command:
				Expect(terraform.Apply()).NotTo(BeZero())
			})
			It("It acks gates if correct ack is provided", func() {
				server.AppendHandlers(
					// Send acks for all gate agreements
					CombineHandlers(
						VerifyRequest(http.MethodPost, cluster123Route+"/gate_agreements"),
						VerifyJQ(".version_gate.id", "999"),
						RespondWithJSON(http.StatusCreated, `{
							"kind": "VersionGateAgreement",
							"id": "888",
							"href": "/api/clusters_mgmt/v1/clusters/24g9q8jhdhv66fi41jfiuup5lsvu61fi/gate_agreements/d2e8d371-1033-11ee-9f05-0a580a820bdb",
							"version_gate": {
							"id": "999",
							"version_raw_id_prefix": "4.14",
							"label": "api.openshift.com/gate-sts",
							"value": "4.14",
							"warning_message": "blah blah blah",
							"description": "whatever",
							"documentation_url": "https://access.redhat.com/solutions/0000000",
							"sts_only": false,
							"creation_timestamp": "2023-04-03T06:39:57.057613Z"
							},
							"creation_timestamp": "2023-06-21T13:02:06.291443Z"
						}`),
					),
					// Create an upgrade policy
					CombineHandlers(
						VerifyRequest(http.MethodPost, cluster123Route+"/node_pools/pool1/upgrade_policies"),
						VerifyJQ(".version", "4.14.1"),
						RespondWithJSON(http.StatusCreated, `
						{
							"id": "123",
							"schedule_type": "manual",
							"upgrade_type": "OSD",
							"version": "4.14.1",
							"next_run": "2023-06-09T20:59:00Z",
							"cluster_id": "123",
							"enable_minor_version_upgrades": true
						}`),
					),
					// Patch the cluster (w/ no changes)
					CombineHandlers(
						VerifyRequest(http.MethodPatch, cluster123Route+"/node_pools/pool1"),
						RespondWithJSON(http.StatusOK, `
						{
							"id": "pool1",
							"replicas": 3,
							"subnet": "subnet-123"
						}`),
					),
				)
				terraform.Source(EvaluateTemplate(`
				resource "rhcs_hcp_machine_pool" "{{.PoolId}}" {
					cluster      = "{{.ClusterId}}"
					name         = "{{.PoolId}}"
					aws_node_pool = {
						instance_type = "r5.xlarge"
					}
					replicas     = 3
					subnet_id = "subnet-123"
					autoscaling = {
						enabled = false
					}
					version = "4.14.1"
					upgrade_acknowledgements_for = "4.14"
				}`, "PoolId", poolId, "ClusterId", clusterId))
				Expect(terraform.Apply()).To(BeZero())
			})
		})
	})
})
