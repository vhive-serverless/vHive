#!/bin/bash

# Function to run deploy operation
run_deploy() {
  ./openyurt_deployer deploy
}

# Function to run deploy-e operation
run_deploy_e() {
  ./openyurt_deployer demo-e 
}

# Function to run deploy-c operation
run_deploy_c() {
  ./openyurt_deployer demo-c 
}

# Function to run demo-print operation
run_demo_print() {
  ./openyurt_deployer demo-print 
}

# Run the deploy operations
run_deploy
run_deploy_e
run_deploy_c

# Run the demo-print operation
run_demo_print
