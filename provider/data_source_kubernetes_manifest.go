package provider

import (
	"context"
	"encoding/json"
	"time"

	//"github.com/henderiw-nephio/kform/kform-plugin/kfprotov1/kfplugin1"
	"github.com/henderiw-nephio/kform/kform-sdk-go/pkg/diag"
	"github.com/henderiw-nephio/kform/kform-sdk-go/pkg/schema"
	"github.com/henderiw/logger/log"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func dataSourceKubernetesManifest() *schema.Resource {
	defaultTimout := 5 * time.Minute
	return &schema.Resource{
		ReadContext: dataSourceKubernetesManifestRead,
		Timeouts: &schema.ResourceTimeout{
			Read:    &defaultTimout,
			Default: &defaultTimout,
		},
	}
}

func dataSourceKubernetesManifestRead(ctx context.Context, obj *schema.ResourceObject, meta interface{}) ([]byte, diag.Diagnostics) {
	client := meta.(*Client)

	u := &unstructured.Unstructured{}
	if err := json.Unmarshal(obj.GetObject(), u); err != nil {
		return nil, diag.FromErr(err)
	}

	log := log.FromContext(ctx)
	log.Info("get data", "u", u)

	newObj, err := client.Get(ctx, u, metav1.GetOptions{})
	if err != nil {
		return nil, diag.FromErr(err)
	}
	b, err := json.Marshal(newObj)
	if err != nil {
		return nil, diag.FromErr(err)
	}
	return b, nil
}
