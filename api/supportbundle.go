package api

import (
	"fmt"
	"io"
	"net/http"
	"strconv"

	"k8s.io/apimachinery/pkg/labels"

	"github.com/gorilla/mux"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"

	longhorn "github.com/longhorn/longhorn-manager/k8s/pkg/apis/longhorn/v1beta1"
	"github.com/longhorn/longhorn-manager/types"
	"github.com/longhorn/longhorn-manager/util"
)

func (s *Server) SupportBundleDownload(rw http.ResponseWriter, r *http.Request) error {
	bundleName := mux.Vars(r)["bundleName"]

	retainSb := false
	if retain, err := strconv.ParseBool(r.URL.Query().Get("retain")); err == nil {
		retainSb = retain
	}

	sb, err := s.m.GetSupportBundle(bundleName)
	if err != nil {
		util.ResponseError(rw, http.StatusBadRequest, errors.Wrap(err, "fail to get support bundle resource"))
		return err
	}

	if sb.Status.State != types.SupportBundleStateReady || sb.Status.FileName == "" || sb.Status.FileSize == 0 {
		util.ResponseError(rw, http.StatusBadRequest, errors.New("support bundle is not ready"))
		return err
	}

	managerPodIP, err := s.getManagerPodIP(sb)
	if err != nil {
		util.ResponseError(rw, http.StatusBadRequest, errors.Wrap(err, "fail to get support bundle manager pod IP"))
		return err
	}

	url := fmt.Sprintf("http://%s:8080/bundle", managerPodIP)
	req, err := http.NewRequestWithContext(r.Context(), http.MethodGet, url, nil)
	if err != nil {
		util.ResponseError(rw, http.StatusInternalServerError, err)
		return err
	}
	resp, err := s.httpClient.Do(req)
	if err != nil {
		util.ResponseError(rw, http.StatusInternalServerError, err)
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		util.ResponseErrorMsg(rw, http.StatusInternalServerError, fmt.Sprintf("unexpected status code %d", resp.StatusCode))
		return err
	}

	rw.Header().Set("Content-Length", fmt.Sprint(sb.Status.FileSize))
	rw.Header().Set("Content-Disposition", "attachment; filename="+sb.Status.FileName)
	contentType := resp.Header.Get("Content-Type")
	if contentType != "" {
		rw.Header().Set("Content-Type", contentType)
	}

	if _, err := io.Copy(rw, resp.Body); err != nil {
		util.ResponseError(rw, http.StatusInternalServerError, err)
		return err
	}

	if retainSb {
		return err
	}

	logrus.Infof("delete support bundle %s", sb.Name)
	err = s.m.DeleteSupportBundle(bundleName)
	if err != nil {
		logrus.Errorf("fail to delete support bundle %s: %s", sb.Name, err)
		return err
	}
	return nil
}

func (s *Server) getManagerPodIP(sb *longhorn.SupportBundle) (string, error) {
	sets := labels.Set{
		"app":                       types.SupportBundleManager,
		types.SupportBundleLabelKey: sb.Name,
	}

	pods, err := s.m.ListPods(sets.AsSelector())
	if err != nil {
		return "", err

	}
	if len(pods) != 1 {
		return "", errors.New("more than one manager pods are found")
	}
	return pods[0].Status.PodIP, nil
}
