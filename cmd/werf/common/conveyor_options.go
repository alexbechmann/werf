package common

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"text/template"

	"github.com/werf/logboek"
	"github.com/werf/werf/pkg/build"
	"github.com/werf/werf/pkg/build/stage"
	"github.com/werf/werf/pkg/config"
	"github.com/werf/werf/pkg/container_backend"
	"github.com/werf/werf/pkg/giterminism_manager"
	"github.com/werf/werf/pkg/image"
	"github.com/werf/werf/pkg/slug"
	"github.com/werf/werf/pkg/storage"
)

func GetConveyorOptions(commonCmdData *CmdData, imagesToProcess build.ImagesToProcess) build.ConveyorOptions {
	return build.ConveyorOptions{
		LocalGitRepoVirtualMergeOptions: stage.VirtualMergeOptions{
			VirtualMerge: *commonCmdData.VirtualMerge,
		},
		ImagesToProcess: imagesToProcess,
	}
}

func GetConveyorOptionsWithParallel(commonCmdData *CmdData, imagesToProcess build.ImagesToProcess, buildStagesOptions build.BuildOptions) (build.ConveyorOptions, error) {
	conveyorOptions := GetConveyorOptions(commonCmdData, imagesToProcess)
	conveyorOptions.Parallel = !(buildStagesOptions.ImageBuildOptions.IntrospectAfterError || buildStagesOptions.ImageBuildOptions.IntrospectBeforeError || len(buildStagesOptions.Targets) != 0) && *commonCmdData.Parallel

	parallelTasksLimit, err := GetParallelTasksLimit(commonCmdData)
	if err != nil {
		return conveyorOptions, fmt.Errorf("getting parallel tasks limit failed: %w", err)
	}

	conveyorOptions.ParallelTasksLimit = parallelTasksLimit

	return conveyorOptions, nil
}

func GetShouldBeBuiltOptions(commonCmdData *CmdData, giterminismManager giterminism_manager.Interface, werfConfig *config.WerfConfig) (options build.ShouldBeBuiltOptions, err error) {
	customTagFuncList, err := getCustomTagFuncList(getCustomTagOptionValues(commonCmdData), commonCmdData, giterminismManager, werfConfig)
	if err != nil {
		return options, err
	}

	options = build.ShouldBeBuiltOptions{CustomTagFuncList: customTagFuncList}
	return options, nil
}

func GetBuildOptions(ctx context.Context, commonCmdData *CmdData, giterminismManager giterminism_manager.Interface, werfConfig *config.WerfConfig) (buildOptions build.BuildOptions, err error) {
	introspectOptions, err := GetIntrospectOptions(commonCmdData, werfConfig)
	if err != nil {
		return buildOptions, err
	}

	var buildReportPath string
	if commonCmdData.BuildReportPath != nil && *commonCmdData.BuildReportPath != "" && commonCmdData.DeprecatedReportPath != nil && *commonCmdData.DeprecatedReportPath != "" {
		return buildOptions, fmt.Errorf("you can't use both --report-path ($WERF_REPORT_PATH) and --build-report-path ($WERF_BUILD_REPORT_PATH), use only the latter instead")
	} else if commonCmdData.BuildReportPath != nil && *commonCmdData.BuildReportPath != "" {
		buildReportPath = *commonCmdData.BuildReportPath
	} else if commonCmdData.DeprecatedReportPath != nil && *commonCmdData.DeprecatedReportPath != "" {
		logboek.Context(ctx).Warn().LogF("DEPRECATED: use --build-report-path ($WERF_BUILD_REPORT_PATH) instead of --report-path ($WERF_REPORT_PATH)\n")
		buildReportPath = *commonCmdData.DeprecatedReportPath
	}

	var buildReportFormat build.ReportFormat
	if commonCmdData.BuildReportFormat != nil && *commonCmdData.BuildReportFormat != "" && commonCmdData.DeprecatedReportFormat != nil && *commonCmdData.DeprecatedReportFormat != "" {
		return buildOptions, fmt.Errorf("you can't use both --report-format ($WERF_REPORT_FORMAT) and --build-report-format ($WERF_BUILD_REPORT_FORMAT), use only the latter instead")
	} else if commonCmdData.BuildReportFormat != nil && *commonCmdData.BuildReportFormat != "" {
		buildReportFormat, err = GetBuildReportFormat(commonCmdData)
		if err != nil {
			return buildOptions, fmt.Errorf("error getting build report format: %w", err)
		}
	} else if commonCmdData.DeprecatedReportFormat != nil && *commonCmdData.DeprecatedReportFormat != "" {
		logboek.Context(ctx).Warn().LogF("DEPRECATED: use --build-report-format ($WERF_BUILD_REPORT_FORMAT) instead of --report-format ($WERF_REPORT_FORMAT)\n")
		buildReportFormat, err = GetDeprecatedReportFormat(commonCmdData)
		if err != nil {
			return buildOptions, fmt.Errorf("error getting report format: %w", err)
		}
	} else {
		buildReportFormat = build.ReportJSON
	}

	customTagFuncList, err := getCustomTagFuncList(getCustomTagOptionValues(commonCmdData), commonCmdData, giterminismManager, werfConfig)
	if err != nil {
		return buildOptions, err
	}

	buildOptions = build.BuildOptions{
		SkipImageMetadataPublication: *commonCmdData.Dev,
		CustomTagFuncList:            customTagFuncList,
		ImageBuildOptions: container_backend.BuildOptions{
			IntrospectAfterError:  *commonCmdData.IntrospectAfterError,
			IntrospectBeforeError: *commonCmdData.IntrospectBeforeError,
		},
		IntrospectOptions: introspectOptions,
		ReportPath:        buildReportPath,
		ReportFormat:      buildReportFormat,
	}

	return buildOptions, nil
}

func getCustomTagFuncList(tagOptionValues []string, commonCmdData *CmdData, giterminismManager giterminism_manager.Interface, werfConfig *config.WerfConfig) ([]image.CustomTagFunc, error) {
	if len(tagOptionValues) == 0 {
		return nil, nil
	}

	if *commonCmdData.Repo.Address == "" || *commonCmdData.Repo.Address == storage.LocalStorageAddress {
		return nil, fmt.Errorf("custom tags can only be used with remote storage: --repo=ADDRESS param required")
	}

	templateName := "--add/use-custom-tag"
	tmpl := template.New(templateName).Delims("%", "%")
	tmpl = tmpl.Funcs(map[string]interface{}{
		"image":                   func() string { return "%[1]s" },
		"image_slug":              func() string { return "%[2]s" },
		"image_safe_slug":         func() string { return "%[3]s" },
		"image_content_based_tag": func() string { return "%[4]s" },
	})

	var tagFuncList []image.CustomTagFunc
	for _, optionValue := range tagOptionValues {
		tmpl, err := tmpl.Parse(optionValue)
		if err != nil {
			return nil, fmt.Errorf("invalid custom tag %q: %w", optionValue, err)
		}

		buf := bytes.NewBuffer(nil)
		if err := tmpl.ExecuteTemplate(buf, templateName, nil); err != nil {
			return nil, fmt.Errorf("invalid custom tag %q: %w", optionValue, err)
		}

		tagOrFormat := buf.String()
		tagFunc := func(imageName, contentBasedTag string) string {
			if strings.ContainsRune(tagOrFormat, '%') {
				return fmt.Sprintf(tagOrFormat, imageName, slug.Slug(imageName), slug.DockerTag(imageName), contentBasedTag)
			} else {
				return tagOrFormat
			}
		}

		contentBasedTagStub := strings.Repeat("x", 70) // 1b77754d35b0a3e603731828ee6f2400c4f937382874db2566c616bb-1624991915332
		var prevImageTag string
		for _, img := range werfConfig.GetAllImages() {
			imageTag := tagFunc(img.GetName(), contentBasedTagStub)

			if err := slug.ValidateDockerTag(imageTag); err != nil {
				return nil, fmt.Errorf("invalid custom tag %q: %w", optionValue, err)
			}

			if prevImageTag == "" {
				prevImageTag = imageTag
				continue
			} else if imageTag == prevImageTag {
				return nil, fmt.Errorf("invalid custom tag %q: it is necessary to use the image name in the tag format if there is more than one image in the werf config (e.g., %q)", tagOrFormat, fmt.Sprintf("%s-%s", "%image%", tagOrFormat))
			}
		}

		tagFuncList = append(tagFuncList, tagFunc)
	}

	return tagFuncList, nil
}

func GetUseCustomTagFunc(commonCmdData *CmdData, giterminismManager giterminism_manager.Interface, werfConfig *config.WerfConfig) (image.CustomTagFunc, error) {
	var tagOptionValues []string
	if *commonCmdData.UseCustomTag != "" {
		tagOptionValues = []string{*commonCmdData.UseCustomTag}
	}

	customTagFuncList, err := getCustomTagFuncList(tagOptionValues, commonCmdData, giterminismManager, werfConfig)
	if err != nil {
		return nil, err
	}

	if len(customTagFuncList) == 0 {
		return nil, nil
	}

	if err := giterminismManager.Inspector().InspectCustomTags(); err != nil {
		return nil, err
	}

	if len(customTagFuncList) != 1 {
		panic("unexpected condition")
	}

	return customTagFuncList[0], nil
}

func getCustomTagOptionValues(commonCmdData *CmdData) []string {
	var tagOptionValues []string

	if commonCmdData.UseCustomTag != nil && *commonCmdData.UseCustomTag != "" {
		tagOptionValues = append(tagOptionValues, *commonCmdData.UseCustomTag)
	}

	if commonCmdData.AddCustomTag != nil {
		tagOptionValues = append(tagOptionValues, getAddCustomTag(commonCmdData)...)
	}

	return tagOptionValues
}
