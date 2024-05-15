package provider

import (
	"context"
	"encoding/json"
	"time"

	"github.com/henderiw/logger/log"
	"github.com/kform-dev/kform-sdk-go/pkg/diag"
	"github.com/kform-dev/kform-sdk-go/pkg/schema"
	"github.com/kform-providers/kubernetes/provider/client"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func dataSourcesKubernetesManifest() *schema.Resource {
	defaultTimout := 5 * time.Minute
	return &schema.Resource{
		ListContext: dataSourcesKubernetesManifestList,
		Timeouts: &schema.ResourceTimeout{
			Read:    &defaultTimout,
			Default: &defaultTimout,
		},
	}
}

func dataSourcesKubernetesManifestList(ctx context.Context, d *schema.ResourceObject, meta interface{}) ([]byte, diag.Diagnostics) {
	client := meta.(client.Client)
	log := log.FromContext(ctx)
	log.Info("list resources")

	//return d.GetData(), nil

	ul := unstructured.UnstructuredList{}
	if err := json.Unmarshal(d.GetObject(), &ul); err != nil {
		return nil, diag.FromErr(err)
	}

	newul := unstructured.UnstructuredList{}
	newul.SetAPIVersion(ul.GetAPIVersion())
	newul.SetKind(ul.GetKind())
	if err := client.List(ctx, &newul); err != nil {
		return nil, diag.FromErr(err)
	}

	b, err := json.Marshal(newul)
	if err != nil {
		return nil, diag.FromErr(err)
	}
	return b, nil
}
