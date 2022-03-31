/*
Copyright 2021 The Crossplane Authors.

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

package config

import (
	"strings"

	"github.com/crossplane-contrib/provider-jet-aws4/config/common"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/pkg/errors"

	tjconfig "github.com/crossplane/terrajet/pkg/config"
	"github.com/crossplane/terrajet/pkg/types/comments"
	"github.com/crossplane/terrajet/pkg/types/name"
)

// GroupKindCalculator returns the correct group and kind name for given TF
// resource.
type GroupKindCalculator func(resource string) (string, string)

// GroupMap contains all overrides we'd like to make to the default group search.
// It's written with data from TF Provider AWS repo service grouping in here:
// https://github.com/hashicorp/terraform-provider-aws/tree/main/internal/service
//
// At the end, all of them are based on grouping of the AWS Go SDK.
// The initial grouping is calculated based on folder grouping of AWS TF Provider
// which is based on Go SDK. Here is the script used to fetch that list:
// https://gist.github.com/muvaf/8d61365ffc1df7757864422ba16d7819
var GroupMap = map[string]GroupKindCalculator{
	"aws_route53_resolver_rule":             ReplaceGroupWords("route53resolver", 2),
	"aws_route53_resolver_rule_association": ReplaceGroupWords("route53resolver", 2),
	"aws_route_table":                       ReplaceGroupWords("ec2", 0),
}

// ReplaceGroupWords uses given group as the group of the resource and removes
// a number of words in resource name before calculating the kind of the resource.
func ReplaceGroupWords(group string, count int) GroupKindCalculator {
	return func(resource string) (string, string) {
		// "aws_route53_resolver_rule": "route53resolver" -> (route53resolver, Rule)
		words := strings.Split(strings.TrimPrefix(resource, "aws_"), "_")
		snakeKind := strings.Join(words[count:], "_")
		return group, name.NewFromSnake(snakeKind).Camel
	}
}

// KindMap contains kind string overrides.
var KindMap = map[string]string{
	"aws_autoscaling_group":                    "AutoscalingGroup",
	"aws_cloudformation_type":                  "CloudFormationType",
	"aws_config_configuration_recorder_status": "AWSConfigurationRecorderStatus",
	"aws_cloudtrail":                           "Trail",
}

// GroupKindOverrides overrides the group and kind of the resource if it matches
// any entry in the GroupMap.
func GroupKindOverrides() tjconfig.ResourceOption {
	return func(r *tjconfig.Resource) {
		if f, ok := GroupMap[r.Name]; ok {
			r.ShortGroup, r.Kind = f(r.Name)
		}
	}
}

// KindOverrides overrides the kind of the resources given in KindMap.
func KindOverrides() tjconfig.ResourceOption {
	return func(r *tjconfig.Resource) {
		if k, ok := KindMap[r.Name]; ok {
			r.Kind = k
		}
	}
}

// RegionAddition adds region to the spec of all resources except iam group which
// does not have a region notion.
func RegionAddition() tjconfig.ResourceOption {
	return func(r *tjconfig.Resource) {
		if r.ShortGroup == "iam" {
			return
		}
		c := "Region is the region you'd like your resource to be created in.\n"
		comment, err := comments.New(c, comments.WithTFTag("-"))
		if err != nil {
			panic(errors.Wrap(err, "cannot build comment for region"))
		}
		r.TerraformResource.Schema["region"] = &schema.Schema{
			Type:        schema.TypeString,
			Required:    true,
			Description: comment.String(),
		}
	}
}

// TagsAllRemoval removes the tags_all field that is used only in tfstate to
// accumulate provider-wide default tags in TF, which is not something we support.
// So, we don't need it as a parameter while "tags" is already in place.
func TagsAllRemoval() tjconfig.ResourceOption {
	return func(r *tjconfig.Resource) {
		if t, ok := r.TerraformResource.Schema["tags_all"]; ok {
			t.Computed = true
			t.Optional = false
		}
	}
}

// IdentifierAssignedByAWS will work for all AWS types because even if the ID
// is assigned by user, we'll see it in the TF State ID.
// The resource-specific configurations should override this whenever possible.
func IdentifierAssignedByAWS() tjconfig.ResourceOption {
	return func(r *tjconfig.Resource) {
		r.ExternalName = tjconfig.IdentifierFromProvider
	}
}

// NamePrefixRemoval makes sure we remove name_prefix from all since it is mostly
// for Terraform functionality.
func NamePrefixRemoval() tjconfig.ResourceOption {
	return func(r *tjconfig.Resource) {
		for _, f := range r.ExternalName.OmittedFields {
			if f == "name_prefix" {
				return
			}
		}
		r.ExternalName.OmittedFields = append(r.ExternalName.OmittedFields, "name_prefix")
	}
}

// KnownReferencers adds referencers for fields that are known and common among
// more than a few resources.
// TODO mleahu: review them
func KnownReferencers() tjconfig.ResourceOption { //nolint:gocyclo
	return func(r *tjconfig.Resource) {
		for k, s := range r.TerraformResource.Schema {
			// We shouldn't add referencers for status fields and sensitive fields
			// since they already have secret referencer.
			if (s.Computed && !s.Optional) || s.Sensitive {
				continue
			}
			switch {
			case strings.HasSuffix(k, "role_arn"):
				r.References[k] = tjconfig.Reference{
					Type:      "github.com/crossplane-contrib/provider-jet-aws/apis/iam/v1alpha2.Role",
					Extractor: common.PathARNExtractor,
				}
			case strings.HasSuffix(k, "security_group_ids"):
				r.References[k] = tjconfig.Reference{
					Type:              "github.com/crossplane-contrib/provider-jet-aws/apis/ec2/v1alpha2.SecurityGroup",
					RefFieldName:      strings.TrimSuffix(name.NewFromSnake(k).Camel, "s") + "Refs",
					SelectorFieldName: strings.TrimSuffix(name.NewFromSnake(k).Camel, "s") + "Selector",
				}
			}
			switch k {
			case "vpc_id":
				r.References["vpc_id"] = tjconfig.Reference{
					Type:              "github.com/crossplane-contrib/provider-jet-aws/apis/ec2/v1alpha2.VPC",
					RefFieldName:      "VpcIdRef",
					SelectorFieldName: "VpcIdSelector",
				}
				if r.ShortGroup == "ec2" {
					// TODO(muvaf): Angryjet should work with the full type path
					// even when it's its own type, but it doesn't for some
					// reason and this is a workaround.
					r.References["vpc_id"] = tjconfig.Reference{
						Type:              "VPC",
						RefFieldName:      "VpcIdRef",
						SelectorFieldName: "VpcIdSelector",
					}
				}
			case "subnet_ids":
				r.References["subnet_ids"] = tjconfig.Reference{
					Type:              "github.com/crossplane-contrib/provider-jet-aws/apis/ec2/v1alpha2.Subnet",
					RefFieldName:      "SubnetIdRefs",
					SelectorFieldName: "SubnetIdSelector",
				}
				if r.ShortGroup == "ec2" {
					// TODO(muvaf): Angryjet should work with the full type path
					// even when it's its own type, but it doesn't for some
					// reason and this is a workaround.
					r.References["subnet_ids"] = tjconfig.Reference{
						Type:              "Subnet",
						RefFieldName:      "SubnetIdRefs",
						SelectorFieldName: "SubnetIdSelector",
					}
				}
			case "subnet_id":
				r.References["subnet_id"] = tjconfig.Reference{
					Type: "github.com/crossplane-contrib/provider-jet-aws/apis/ec2/v1alpha2.Subnet",
				}
			case "security_group_id":
				r.References["security_group_id"] = tjconfig.Reference{
					Type: "github.com/crossplane-contrib/provider-jet-aws/apis/ec2/v1alpha2.SecurityGroup",
				}
			case "kms_key_id":
				r.References["kms_key_id"] = tjconfig.Reference{
					Type: "github.com/crossplane-contrib/provider-jet-aws/apis/kms/v1alpha2.Key",
				}
			case "kms_key_arn":
				r.References["kms_key_arn"] = tjconfig.Reference{
					Type: "github.com/crossplane-contrib/provider-jet-aws/apis/kms/v1alpha2.Key",
				}
			case "kms_key":
				r.References["kms_key"] = tjconfig.Reference{
					Type: "github.com/crossplane-contrib/provider-jet-aws/apis/kms/v1alpha2.Key",
				}
			}
		}
	}
}
