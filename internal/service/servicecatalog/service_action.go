// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package servicecatalog

import (
	"context"
	"log"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/servicecatalog"
	"github.com/aws/aws-sdk-go-v2/service/servicecatalog/types"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/id"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/retry"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/validation"
	"github.com/hashicorp/terraform-provider-aws/internal/conns"
	"github.com/hashicorp/terraform-provider-aws/internal/enum"
	"github.com/hashicorp/terraform-provider-aws/internal/errs"
	"github.com/hashicorp/terraform-provider-aws/internal/errs/sdkdiag"
	"github.com/hashicorp/terraform-provider-aws/internal/tfresource"
)

// @SDKResource("aws_servicecatalog_service_action")
func ResourceServiceAction() *schema.Resource {
	return &schema.Resource{
		CreateWithoutTimeout: resourceServiceActionCreate,
		ReadWithoutTimeout:   resourceServiceActionRead,
		UpdateWithoutTimeout: resourceServiceActionUpdate,
		DeleteWithoutTimeout: resourceServiceActionDelete,
		Importer: &schema.ResourceImporter{
			StateContext: schema.ImportStatePassthroughContext,
		},

		Timeouts: &schema.ResourceTimeout{
			Create: schema.DefaultTimeout(ServiceActionReadyTimeout),
			Read:   schema.DefaultTimeout(ServiceActionReadTimeout),
			Update: schema.DefaultTimeout(ServiceActionUpdateTimeout),
			Delete: schema.DefaultTimeout(ServiceActionDeleteTimeout),
		},

		Schema: map[string]*schema.Schema{
			"accept_language": {
				Type:         schema.TypeString,
				Optional:     true,
				Default:      AcceptLanguageEnglish,
				ValidateFunc: validation.StringInSlice(AcceptLanguage_Values(), false),
			},
			"definition": {
				Type:     schema.TypeList,
				Required: true,
				MaxItems: 1,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"assume_role": { // ServiceActionDefinitionKeyAssumeRole
							Type:     schema.TypeString,
							Optional: true,
						},
						"name": { // ServiceActionDefinitionKeyName
							Type:     schema.TypeString,
							Required: true,
						},
						"parameters": { // ServiceActionDefinitionKeyParameters
							Type:             schema.TypeString,
							Optional:         true,
							ValidateFunc:     validation.StringIsJSON,
							DiffSuppressFunc: suppressEquivalentJSONEmptyNilDiffs,
						},
						"type": {
							Type:             schema.TypeString,
							Optional:         true,
							Default:          types.ServiceActionDefinitionTypeSsmAutomation,
							ForceNew:         true,
							ValidateDiagFunc: enum.Validate[types.ServiceActionDefinitionType](),
						},
						"version": { // ServiceActionDefinitionKeyVersion
							Type:     schema.TypeString,
							Required: true,
						},
					},
				},
			},
			"description": {
				Type:     schema.TypeString,
				Optional: true,
				Computed: true,
			},
			"name": {
				Type:     schema.TypeString,
				Required: true,
			},
		},
	}
}

func resourceServiceActionCreate(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	var diags diag.Diagnostics
	conn := meta.(*conns.AWSClient).ServiceCatalogClient(ctx)

	input := &servicecatalog.CreateServiceActionInput{
		IdempotencyToken: aws.String(id.UniqueId()),
		Name:             aws.String(d.Get("name").(string)),
		Definition:       expandServiceActionDefinition(d.Get("definition").([]interface{})[0].(map[string]interface{})),
		DefinitionType:   types.ServiceActionDefinitionType(d.Get("definition.0.type").(string)),
	}

	if v, ok := d.GetOk("accept_language"); ok {
		input.AcceptLanguage = aws.String(v.(string))
	}

	if v, ok := d.GetOk("description"); ok {
		input.Description = aws.String(v.(string))
	}

	var output *servicecatalog.CreateServiceActionOutput
	err := retry.RetryContext(ctx, d.Timeout(schema.TimeoutCreate), func() *retry.RetryError {
		var err error

		output, err = conn.CreateServiceAction(ctx, input)

		if errs.Contains(err, "profile does not exist") {
			return retry.RetryableError(err)
		}

		if err != nil {
			return retry.NonRetryableError(err)
		}

		return nil
	})

	if tfresource.TimedOut(err) {
		output, err = conn.CreateServiceAction(ctx, input)
	}

	if err != nil {
		return sdkdiag.AppendErrorf(diags, "creating Service Catalog Service Action: %s", err)
	}

	if output == nil || output.ServiceActionDetail == nil || output.ServiceActionDetail.ServiceActionSummary == nil {
		return sdkdiag.AppendErrorf(diags, "creating Service Catalog Service Action: empty response")
	}

	d.SetId(aws.ToString(output.ServiceActionDetail.ServiceActionSummary.Id))

	return append(diags, resourceServiceActionRead(ctx, d, meta)...)
}

func resourceServiceActionRead(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	var diags diag.Diagnostics
	conn := meta.(*conns.AWSClient).ServiceCatalogClient(ctx)

	output, err := WaitServiceActionReady(ctx, conn, d.Get("accept_language").(string), d.Id(), d.Timeout(schema.TimeoutRead))

	if !d.IsNewResource() && tfresource.NotFound(err) {
		log.Printf("[WARN] Service Catalog Service Action (%s) not found, removing from state", d.Id())
		d.SetId("")
		return diags
	}

	if err != nil {
		return sdkdiag.AppendErrorf(diags, "describing Service Catalog Service Action (%s): %s", d.Id(), err)
	}

	if output == nil || output.ServiceActionSummary == nil {
		return sdkdiag.AppendErrorf(diags, "getting Service Catalog Service Action (%s): empty response", d.Id())
	}

	sas := output.ServiceActionSummary

	d.Set("description", sas.Description)
	d.Set("name", sas.Name)

	if output.Definition != nil {
		d.Set("definition", []interface{}{flattenServiceActionDefinition(output.Definition, aws.ToString(sas.DefinitionType))})
	} else {
		d.Set("definition", nil)
	}

	return diags
}

func resourceServiceActionUpdate(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	var diags diag.Diagnostics
	conn := meta.(*conns.AWSClient).ServiceCatalogClient(ctx)

	input := &servicecatalog.UpdateServiceActionInput{
		Id: aws.String(d.Id()),
	}

	if d.HasChange("accept_language") {
		input.AcceptLanguage = aws.String(d.Get("accept_language").(string))
	}

	if d.HasChange("definition") {
		input.Definition = expandServiceActionDefinition(d.Get("definition").([]interface{})[0].(map[string]interface{}))
	}

	if d.HasChange("description") {
		input.Description = aws.String(d.Get("description").(string))
	}

	if d.HasChange("name") {
		input.Name = aws.String(d.Get("name").(string))
	}

	err := retry.RetryContext(ctx, d.Timeout(schema.TimeoutUpdate), func() *retry.RetryError {
		_, err := conn.UpdateServiceAction(ctx, input)

		if errs.Contains(err, "profile does not exist") {
			return retry.RetryableError(err)
		}

		if err != nil {
			return retry.NonRetryableError(err)
		}

		return nil
	})

	if tfresource.TimedOut(err) {
		_, err = conn.UpdateServiceAction(ctx, input)
	}

	if err != nil {
		return sdkdiag.AppendErrorf(diags, "updating Service Catalog Service Action (%s): %s", d.Id(), err)
	}

	return append(diags, resourceServiceActionRead(ctx, d, meta)...)
}

func resourceServiceActionDelete(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	var diags diag.Diagnostics
	conn := meta.(*conns.AWSClient).ServiceCatalogClient(ctx)

	input := &servicecatalog.DeleteServiceActionInput{
		Id: aws.String(d.Id()),
	}

	err := retry.RetryContext(ctx, d.Timeout(schema.TimeoutDelete), func() *retry.RetryError {
		_, err := conn.DeleteServiceAction(ctx, input)

		if errs.IsA[*types.ResourceInUseException](err) {
			return retry.RetryableError(err)
		}

		if err != nil {
			return retry.NonRetryableError(err)
		}

		return nil
	})

	if tfresource.TimedOut(err) {
		_, err = conn.DeleteServiceAction(ctx, input)
	}

	if errs.IsA[*types.ResourceNotFoundException](err) {
		log.Printf("[INFO] Attempted to delete Service Action (%s) but does not exist", d.Id())
		return diags
	}

	if err != nil {
		return sdkdiag.AppendErrorf(diags, "deleting Service Catalog Service Action (%s): %s", d.Id(), err)
	}

	if err := WaitServiceActionDeleted(ctx, conn, d.Get("accept_language").(string), d.Id(), d.Timeout(schema.TimeoutDelete)); err != nil {
		return sdkdiag.AppendErrorf(diags, "waiting for Service Catalog Service Action (%s) to be deleted: %s", d.Id(), err)
	}

	return diags
}

func expandServiceActionDefinition(tfMap map[string]interface{}) map[string]*string {
	if tfMap == nil {
		return nil
	}

	apiObject := make(map[string]*string)

	if v, ok := tfMap["assume_role"].(string); ok && v != "" {
		apiObject[string(types.ServiceActionDefinitionKeyAssumeRole)] = aws.String(v)
	}

	if v, ok := tfMap["name"].(string); ok && v != "" {
		apiObject[string(types.ServiceActionDefinitionKeyName)] = aws.String(v)
	}

	if v, ok := tfMap["parameters"].(string); ok && v != "" {
		apiObject[string(types.ServiceActionDefinitionKeyParameters)] = aws.String(v)
	}

	if v, ok := tfMap["version"].(string); ok && v != "" {
		apiObject[string(types.ServiceActionDefinitionKeyVersion)] = aws.String(v)
	}

	return apiObject
}

func flattenServiceActionDefinition(apiObject map[string]*string, definitionType string) map[string]interface{} {
	if apiObject == nil {
		return nil
	}

	tfMap := map[string]interface{}{}

	if v, ok := apiObject[string(types.ServiceActionDefinitionKeyAssumeRole)]; ok && v != nil {
		tfMap["assume_role"] = aws.ToString(v)
	}

	if v, ok := apiObject[string(types.ServiceActionDefinitionKeyName)]; ok && v != nil {
		tfMap["name"] = aws.ToString(v)
	}

	if v, ok := apiObject[string(types.ServiceActionDefinitionKeyParameters)]; ok && v != nil {
		tfMap["parameters"] = aws.ToString(v)
	}

	if v, ok := apiObject[string(types.ServiceActionDefinitionKeyVersion)]; ok && v != nil {
		tfMap["version"] = aws.ToString(v)
	}

	if definitionType != "" {
		tfMap["type"] = definitionType
	}

	return tfMap
}
