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
	// Note(turkenh): we are importing this to embed provider schema document
	_ "embed"

	tjconfig "github.com/crossplane/terrajet/pkg/config"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"

	"github.com/crossplane-contrib/provider-jet-aws4/config/servicecatalog"
)

const (
	resourcePrefix = "aws4"
	modulePath     = "github.com/crossplane-contrib/provider-jet-aws4"
)

// IncludedResources lists all resource patterns included in small set release.
var IncludedResources = []string{

	// Service Catalog
	"aws_servicecatalog_provisioned_product$",
}

var skipList = []string{
	/*	"aws_waf_rule_group$",              // Too big CRD schema
		"aws_wafregional_rule_group$",      // Too big CRD schema
		"aws_glue_connection$",             // See https://github.com/crossplane-contrib/terrajet/issues/100
		"aws_mwaa_environment$",            // See https://github.com/crossplane-contrib/terrajet/issues/100
		"aws_ecs_tag$",                     // tags are already managed by ecs resources.
		"aws_alb$",                         // identical with aws_lb
		"aws_alb_target_group_attachment$", // identical with aws_lb_target_group_attachment
		"aws_iam_policy_attachment$",       // identical with aws_iam_*_policy_attachment resources.
		"aws_iam_group_policy$",            // identical with aws_iam_*_policy_attachment resources.
		"aws_iam_role_policy$",             // identical with aws_iam_*_policy_attachment resources.
		"aws_iam_user_policy$",             // identical with aws_iam_*_policy_attachment resources.
	*/}

//go:embed schema.json
var providerSchema string

// GetProvider returns provider configuration
func GetProvider() *tjconfig.Provider {
	defaultResourceFn := func(name string, terraformResource *schema.Resource, opts ...tjconfig.ResourceOption) *tjconfig.Resource {
		r := tjconfig.DefaultResource(name, terraformResource,
			GroupKindOverrides(),
			KindOverrides(),
			RegionAddition(),
			TagsAllRemoval(),
			IdentifierAssignedByAWS(),
			NamePrefixRemoval(),
			KnownReferencers(),
		)
		// Add any provider-specific defaulting here. For example:
		//   r.ExternalName = tjconfig.IdentifierFromProvider
		return r
	}

	pc := tjconfig.NewProviderWithSchema([]byte(providerSchema), resourcePrefix, modulePath,
		tjconfig.WithShortName("awsjet"),
		tjconfig.WithRootGroup("aws.jet.crossplane.io"),
		tjconfig.WithIncludeList(IncludedResources),
		tjconfig.WithSkipList(skipList),
		tjconfig.WithDefaultResourceFn(defaultResourceFn))

	for _, configure := range []func(provider *tjconfig.Provider){
		// add custom config functions
		servicecatalog.Configure,
	} {
		configure(pc)
	}

	pc.ConfigureResources()
	return pc
}
