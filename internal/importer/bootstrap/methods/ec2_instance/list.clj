(defn run [args]
  ;; Simulated EC2 list — returns mock instances
  [{:InstanceId "i-001" :InstanceType "t3.micro" :State {:Name "running" :Code 16}}
   {:InstanceId "i-002" :InstanceType "t3.small" :State {:Name "stopped" :Code 80}}])
