package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/henderiw/logger/log"
	"github.com/kform-dev/kform-sdk-go/pkg/diag"
	"github.com/kform-dev/kform-sdk-go/pkg/schema"
	"github.com/kform-providers/kubernetes/provider/kstatus/status"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
)

func resourceKubernetesManifest() *schema.Resource {
	defaultTimout := 5 * time.Minute
	return &schema.Resource{
		ReadContext:   resourceKubernetesManifestRead,
		CreateContext: resourceKubernetesManifestCreate,
		UpdateContext: resourceKubernetesManifestUpdate,
		DeleteContext: resourceKubernetesManifestDelete,
		Timeouts: &schema.ResourceTimeout{
			Create:  &defaultTimout,
			Read:    &defaultTimout,
			Default: &defaultTimout,
		},
	}
}

func resourceKubernetesManifestRead(ctx context.Context, obj *schema.ResourceObject, meta interface{}) ([]byte, diag.Diagnostics) {
	client := meta.(*Client)

	u := &unstructured.Unstructured{}
	if err := json.Unmarshal(obj.GetObject(), u); err != nil {
		return nil, diag.FromErr(err)
	}

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

func resourceKubernetesManifestCreate(ctx context.Context, obj *schema.ResourceObject, meta interface{}) ([]byte, diag.Diagnostics) {
	client := meta.(*Client)

	u := &unstructured.Unstructured{}
	if err := json.Unmarshal(obj.GetObject(), u); err != nil {
		return nil, diag.FromErr(err)
	}

	var dryRun []string
	if obj.IsDryRun() {
		dryRun = []string{"All"}
	}

	newObj, err := client.Create(ctx, u, metav1.CreateOptions{DryRun: dryRun})
	if err != nil {
		return nil, diag.FromErr(err)
	}

	// when dryrun we do not get the response from the system as we already got the data
	if obj.IsDryRun() {
		b, err := json.Marshal(newObj)
		if err != nil {
			return nil, diag.FromErr(err)
		}
		return b, nil
	}

	// when no dryrun, we get the response from the system by checking the status
	newObj, err = getStatusWithRetries(ctx, client, u, false)
	if err != nil {
		return nil, diag.FromErr(err)
	}
	b, err := json.Marshal(newObj)
	if err != nil {
		return nil, diag.FromErr(err)
	}
	return b, nil
}

func resourceKubernetesManifestUpdate(ctx context.Context, obj *schema.ResourceObject, meta interface{}) ([]byte, diag.Diagnostics) {
	client := meta.(*Client)

	newu := &unstructured.Unstructured{}
	if err := json.Unmarshal(obj.GetObject(), newu); err != nil {
		return nil, diag.FromErr(err)
	}

	oldu := &unstructured.Unstructured{}
	if err := json.Unmarshal(obj.GetOldObject(), oldu); err != nil {
		return nil, diag.FromErr(err)
	}
	if oldu.GetResourceVersion() != "" {
		newu.SetResourceVersion(oldu.GetResourceVersion())
	}

	var dryRun []string
	if obj.IsDryRun() {
		dryRun = []string{"All"}
	}

	newObj, err := client.Update(ctx, newu, metav1.UpdateOptions{DryRun: dryRun})
	if err != nil {
		return nil, diag.FromErr(err)
	}

	// when dryrun we do not get the response from the system as we already got the data
	if obj.IsDryRun() {
		b, err := json.Marshal(newObj)
		if err != nil {
			return nil, diag.FromErr(err)
		}
		return b, nil
	}

	// when no dryrun, we get the response from the system by checking the status
	newObj, err = getStatusWithRetries(ctx, client, newu, false)
	if err != nil {
		return nil, diag.FromErr(err)
	}

	b, err := json.Marshal(newObj)
	if err != nil {
		return nil, diag.FromErr(err)
	}
	return b, nil
}

func resourceKubernetesManifestDelete(ctx context.Context, obj *schema.ResourceObject, meta interface{}) diag.Diagnostics {
	client := meta.(*Client)

	u := &unstructured.Unstructured{}
	if err := json.Unmarshal(obj.GetObject(), u); err != nil {
		return diag.FromErr(err)
	}

	if _, err := client.Get(ctx, u, metav1.GetOptions{}); err != nil {
		if strings.Contains(err.Error(), "not found") {
			return nil
		}
		return diag.FromErr(err)
	}

	var dryRun []string
	if obj.IsDryRun() {
		dryRun = []string{"All"}
	}

	if err := client.Delete(ctx, u, metav1.DeleteOptions{DryRun: dryRun}); err != nil {
		return diag.FromErr(err)
	}

	if _, err := getStatusWithRetries(ctx, client, u, true); err != nil {
		return diag.FromErr(err)
	}

	return nil
}

const (
	maxRetries                    = 5
	backoffFactor   float64       = 2
	initialDelay    time.Duration = 1 * time.Second
	initialGetDelay time.Duration = 500 * time.Millisecond
)

// getStatusWithRetries tries to get Status with exponential backoff.
// maxRetries: the maximum number of retries before giving up.
// backoffFactor: the factor by which the backoff duration is exponentially increased.
// initialDelay: the initial delay before the first retry.
func getStatusWithRetries(ctx context.Context, client *Client, u *unstructured.Unstructured, delete bool) (*unstructured.Unstructured, error) {
	//log := log.FromContext(ctx)
	gvk := u.GetObjectKind().GroupVersionKind().String()
	nsn := types.NamespacedName{Namespace: u.GetNamespace(), Name: u.GetName()}.String()
	// we wait initially to ensure the status is updated
	// otherwise we might conclude the reconcile is ready
	// while the status is not yet updated
	time.Sleep(initialGetDelay)
	var err error
	for attempt := 0; attempt < maxRetries; attempt++ {
		// get the resource
		newObj, cont, err := getStatus(ctx, client, u, delete, attempt)
		if !cont {
			return newObj, err
		}

		// Calculate the next backoff delay
		backoff := float64(initialDelay) * math.Pow(backoffFactor, float64(attempt))
		backoffDuration := time.Duration(backoff)

		fmt.Printf("getStatus gvk %s nsn %s , retrying in %v... (Attempt %d/%d)\n",
			gvk,
			nsn,
			backoffDuration,
			attempt+1,
			maxRetries,
		)

		// Wait for the backoff duration before retrying
		time.Sleep(backoffDuration)
	}
	return nil, fmt.Errorf("getStatus gvk %s nsn %s after %d retries: %w", gvk, nsn, maxRetries, err)
}

// getStatus gets the status of the object and returns the object if found, a boolean indicating continue true/false
// and an error code
func getStatus(ctx context.Context, client *Client, u *unstructured.Unstructured, delete bool, attempt int) (*unstructured.Unstructured, bool, error) {
	log := log.FromContext(ctx)
	newObj, err := client.Get(ctx, u, metav1.GetOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			if delete {
				// success delete
				return nil, false, nil
			}
			// we should continue
			return nil, true, nil
		}
		log.Error("cannot get object", "err", err)
		return nil, true, err
	}
	result, err := status.Compute(newObj)
	if err != nil {
		log.Error("cannot get object", "err", err)
		return newObj, true, err
	}
	if result.Status == metav1.ConditionFalse {
		if result.Reason == status.ReasonFailed {
			err := fmt.Errorf("failed: %s", result.Message)
			log.Error(err.Error())
			return newObj, false, err
		}
		return newObj, true, nil
	}
	if result.Reason == status.ReasonNoStatusInfo && attempt < maxRetries-2 {
		// continue since we expect status by default - we assume status field will
		// come, so hence we retry maxRetries -2 (which is 4 times), the 5th time we
		// just report ok as we did not get status for some time.
		return newObj, true, nil
	}
	// success (update/create)
	return newObj, false, nil
}
