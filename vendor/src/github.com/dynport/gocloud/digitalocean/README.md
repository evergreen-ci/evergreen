# digo

[![Build Status](https://travis-ci.org/dynport/digo.png?branch=master)](https://travis-ci.org/dynport/digo)

[DigitalOcean](https://www.digitalocean.com/?refcode=b06a6a609632) API cli tool and library for golang

## Requirements

Set the following env variables:
    
    export DIGITAL_OCEAN_CLIENT_ID=<secret>
    export DIGITAL_OCEAN_API_KEY=<secret>

These env settings can be set optionally:

    export DIGITAL_OCEAN_DEFAULT_REGION_ID=2
    export DIGITAL_OCEAN_DEFAULT_SIZE_ID=66
    export DIGITAL_OCEAN_DEFAULT_IMAGE_ID=350076
    export DIGITAL_OCEAN_DEFAULT_SSH_KEY=<secret_id>

## Installation
  
Download and extract the appropriate binaries from [github.com/dynport/digo/releases](https://github.com/dynport/digo/releases) to e.g. `/usr/local/bin/`.

## Usage
    $ digo --help
    USAGE
    droplet	create 	<name>                 	Create new droplet                            
                                            -i DEFAULT: "350076" Image id for new droplet 
                                            -r DEFAULT: "2"      Region id for new droplet
                                            -s DEFAULT: "66"     Size id for new droplet  
                                            -k DEFAULT: "22197"  Ssh key to be used       
    droplet	destroy	<droplet_id>           	Destroy droplet                               
    droplet	info   	                       	Describe Droplet                              
    droplet	list   	                       	List active droplets                          
    droplet	rebuild	<droplet_id>           	Rebuild droplet                               
                                            -i DEFAULT: "0" Rebuild droplet               
    droplet	rename 	<droplet_id> <new_name>	Describe Droplet                              
    image  	list   	                       	List available droplet images                 
    key    	list   	                       	List available ssh keys                       
    region 	list   	                       	List available droplet regions                
    size   	list   	                       	List available droplet sizes                  
    version	       	                       	Print version and revision                    

## Todos (not implemented yet)

Most of the missing functionality should be straight to implement (best with some refactorings to DRY things up).

### Droplets

* reboot
* power_cycle
* shutdown
* power_on
* power_off
* password_reset
* resize
* snapshot
* restore
* enable_backups
* disable_backups

### Images
* show
* destroy
* transfer

### SSH Keys
* new
* show
* edit
* destroy

### Domains

* index
* new
* show
* destroy
* records
* records/new
* records/show
* records/edit
* records/destroy
