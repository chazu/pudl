package examples

import "pudl.schemas/pudl/model"

// EC2 instance resource shape (inline since we don't have real AWS schemas yet)
#EC2InstanceResource: {
	InstanceId:   string
	InstanceType: string
	ImageId:      string
	State: {
		Name: "pending" | "running" | "shutting-down" | "terminated" | "stopping" | "stopped"
		Code: int
	}
	VpcId?:      string
	SubnetId?:   string
	Tags?: [...{
		Key:   string
		Value: string
	}]
	...
}

// EC2 instance state shape (for drift detection)
#EC2InstanceState: {
	InstanceId: string
	State: {
		Name: string
		Code: int
	}
	PublicIpAddress?:  string
	PrivateIpAddress?: string
	...
}

// Full-featured EC2 instance model
#EC2InstanceModel: model.#Model & {
	schema: #EC2InstanceResource
	state:  #EC2InstanceState

	metadata: model.#ModelMetadata & {
		name:        "ec2_instance"
		description: "AWS EC2 compute instance"
		category:    "compute"
		icon:        "server"
	}

	methods: {
		list: model.#Method & {
			kind:        "action"
			description: "List all EC2 instances"
			returns:     [...#EC2InstanceResource]
			timeout:     "30s"
		}
		create: model.#Method & {
			kind:        "action"
			description: "Launch a new EC2 instance"
			inputs: {
				InstanceType: string
				ImageId:      string
				SubnetId?:    string
			}
			returns: #EC2InstanceResource
			timeout: "2m"
			retries: 1
		}
		delete: model.#Method & {
			kind:        "action"
			description: "Terminate an EC2 instance"
			inputs: {
				InstanceId: string
			}
			timeout: "2m"
		}
		valid_credentials: model.#Method & {
			kind:        "qualification"
			description: "Verify AWS credentials are valid"
			returns:     model.#QualificationResult
			blocks:      ["create", "delete", "list"]
		}
		ami_exists: model.#Method & {
			kind:        "qualification"
			description: "Verify the specified AMI exists and is available"
			inputs: {
				ImageId: string
			}
			returns: model.#QualificationResult
			blocks:  ["create"]
		}
	}

	sockets: {
		vpc_id: model.#Socket & {
			direction:   "input"
			type:        string
			description: "VPC ID for instance placement"
			required:    true
		}
		subnet_id: model.#Socket & {
			direction:   "input"
			type:        string
			description: "Subnet ID for instance placement"
		}
		instance_id: model.#Socket & {
			direction:   "output"
			type:        string
			description: "ID of the created/managed instance"
		}
		private_ip: model.#Socket & {
			direction:   "output"
			type:        string
			description: "Private IP address of the instance"
			required:    false
		}
	}

	auth: model.#AuthConfig & {
		method: "sigv4"
	}
}
