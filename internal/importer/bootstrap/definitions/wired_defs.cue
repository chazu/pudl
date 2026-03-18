package definitions

import "pudl.schemas/pudl/model/examples"

// VPC definition provides vpc_id output
prod_vpc: {
	_model: "examples.#VPCModel" // marker for discovery
	outputs: {
		vpc_id: "vpc-abc123"
	}
}

// EC2 instance wires to VPC's output
prod_instance: examples.#EC2InstanceModel & {
	schema: {
		InstanceId:   "i-pending"
		InstanceType: "t3.micro"
		ImageId:      "ami-12345"
		State: {Name: "pending", Code: 0}
		VpcId: prod_vpc.outputs.vpc_id // socket wiring!
	}
}
