/*
Copyright 2021.

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

/*
lr.go
estimate (node/pod) component and total power by linear regression approach when trained model weights are available.
The model weights can be obtained by Kepler Model Server or configured initial model URL.
*/

package regressor

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"

	"github.com/sustainable-computing-io/kepler/pkg/config"
	"github.com/sustainable-computing-io/kepler/pkg/model/types"
	"github.com/sustainable-computing-io/kepler/pkg/sensors/components/source"
)

var (
	SampleDynPowerValue float64 = 100.0

	processFeatureNames = []string{
		config.CPUCycle,
		config.CPUInstruction,
		config.CacheMiss,
	}
	systemMetaDataFeatureNames = []string{"cpu_architecture"}
	processFeatureValues       = [][]float64{
		{1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1}, // process A
		{1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1}, // process B
	}
	nodeFeatureValues           = []float64{2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2}
	systemMetaDataFeatureValues = []string{"Sandy Bridge"}
)

var (
	SampleCategoricalFeatures = map[string]CategoricalFeature{
		"Sandy Bridge": {
			Weight: 1.0,
		},
	}
	SampleCoreNumericalVars = map[string]NormalizedNumericalFeature{
		"cpu_cycles": {Weight: 1.0, Scale: 2},
	}
	SampleDramNumbericalVars = map[string]NormalizedNumericalFeature{
		"cache_miss": {Weight: 1.0, Scale: 2},
	}
	DummyWeightHandler                      = http.HandlerFunc(genHandlerFunc([]float64{}, types.LinearRegressionTrainer))
	ModelCores                              = config.GenerateSpec().Cores
	ExpectedAbsPowerFromDummyWeightHandler  = 2500
	ExpectedIdlePowerFromDummyWeightHandler = 2000
)

func GenPlatformModelWeights(curveFitWeights []float64, trainerName string) ComponentModelWeights {
	return ComponentModelWeights{
		ModelName: trainerName + "_0",
		ModelMachineSpec: &config.MachineSpec{
			Cores: ModelCores,
		},
		Platform: genWeights(SampleCoreNumericalVars, curveFitWeights),
	}
}

func GenComponentModelWeights(curveFitWeights []float64) ComponentModelWeights {
	return ComponentModelWeights{
		Core: genWeights(SampleCoreNumericalVars, curveFitWeights),
		DRAM: genWeights(SampleDramNumbericalVars, curveFitWeights),
	}
}

func genWeights(numericalVars map[string]NormalizedNumericalFeature, curveFitWeights []float64) *ModelWeights {
	return &ModelWeights{
		AllWeights{
			BiasWeight:           1.0,
			CategoricalVariables: map[string]map[string]CategoricalFeature{"cpu_architecture": SampleCategoricalFeatures},
			NumericalVariables:   numericalVars,
			CurveFitWeights:      curveFitWeights,
		},
	}
}

func genHandlerFunc(curvefit []float64, trainerName string) (handlerFunc func(w http.ResponseWriter, r *http.Request)) {
	return func(w http.ResponseWriter, r *http.Request) {
		reqBody, err := io.ReadAll(r.Body)
		if err != nil {
			panic(err)
		}
		var req ModelRequest
		err = json.Unmarshal(reqBody, &req)
		if err != nil {
			panic(err)
		}
		if req.EnergySource == types.ComponentEnergySource {
			err = json.NewEncoder(w).Encode(GenComponentModelWeights(curvefit))
		} else {
			err = json.NewEncoder(w).Encode(GenPlatformModelWeights(curvefit, trainerName))
		}
		if err != nil {
			panic(err)
		}
	}
}

func genRegressor(outputType types.ModelOutputType, energySource, modelServerEndpoint, modelWeightsURL, modelWeightFilepath, trainerName string) Regressor {
	config.SetModelServerEnable(true)
	config.SetModelServerEndpoint(modelServerEndpoint)
	return Regressor{
		ModelServerEndpoint:         modelServerEndpoint,
		OutputType:                  outputType,
		EnergySource:                energySource,
		FloatFeatureNames:           processFeatureNames,
		SystemMetaDataFeatureNames:  systemMetaDataFeatureNames,
		SystemMetaDataFeatureValues: systemMetaDataFeatureValues,
		ModelWeightsURL:             modelWeightsURL,
		ModelWeightsFilepath:        modelWeightFilepath,
		TrainerName:                 trainerName,
		RequestMachineSpec:          config.GetMachineSpec(),
		DiscoveredMachineSpec:       config.GenerateSpec(),
	}
}

func GetNodePlatformPowerFromDummyServer(handler http.HandlerFunc, trainer string) (power []uint64) {
	testServer := httptest.NewServer(handler)
	modelWeightFilepath := config.GetDefaultPowerModelURL(types.AbsPower.String(), types.PlatformEnergySource)
	r := genRegressor(types.AbsPower, types.PlatformEnergySource, testServer.URL, "", modelWeightFilepath, trainer)
	err := r.Start()
	Expect(err).To(BeNil())
	r.ResetSampleIdx()
	r.AddNodeFeatureValues(nodeFeatureValues) // add samples to the power model
	powers, err := r.GetPlatformPower(false)
	Expect(err).NotTo(HaveOccurred())
	Expect(len(powers)).Should(Equal(1))
	return powers
}

func GetNodeComponentsPowerFromDummyServer(handler http.HandlerFunc, trainer string) (compPowers []source.NodeComponentsEnergy) {
	testServer := httptest.NewServer(handler)
	modelWeightFilepath := config.GetDefaultPowerModelURL(types.AbsPower.String(), types.ComponentEnergySource)
	r := genRegressor(types.AbsPower, types.ComponentEnergySource, testServer.URL, "", modelWeightFilepath, trainer)
	err := r.Start()
	Expect(err).To(BeNil())
	r.ResetSampleIdx()
	r.AddNodeFeatureValues(nodeFeatureValues) // add samples to the power model
	compPowers, err = r.GetComponentsPower(false)
	Expect(err).NotTo(HaveOccurred())
	Expect(len(compPowers)).Should(Equal(1))
	return compPowers
}

var _ = Describe("Test Regressor Weight Unit (default trainer)", func() {

	BeforeEach(func() {
		_, err := config.Initialize(".")
		Expect(err).ShouldNot(HaveOccurred())
	})

	Context("with dummy model server", func() {
		It("Get Node Platform Power By Default Regression with ModelServerEndpoint", func() {
			powers := GetNodePlatformPowerFromDummyServer(DummyWeightHandler, "")
			// TODO: verify if the power makes sense
			Expect(powers[0]).Should(BeEquivalentTo(3000))
		})

		It("Get Node Components Power By Default Regression Estimator with ModelServerEndpoint", func() {
			compPowers := GetNodeComponentsPowerFromDummyServer(genHandlerFunc([]float64{}, types.LinearRegressionTrainer), "")
			// TODO: verify if the power makes sense
			Expect(compPowers[0].Core).Should(BeEquivalentTo(3000))
		})

		It("Get Process Platform Power By Default Regression Estimator with ModelServerEndpoint", func() {
			testServer := httptest.NewServer(DummyWeightHandler)
			modelWeightFilepath := config.GetDefaultPowerModelURL(types.DynPower.String(), types.PlatformEnergySource)
			r := genRegressor(types.DynPower, types.PlatformEnergySource, testServer.URL, "", modelWeightFilepath, "")
			err := r.Start()
			Expect(err).To(BeNil())
			r.ResetSampleIdx()
			for _, processFeatureValues := range processFeatureValues {
				r.AddProcessFeatureValues(processFeatureValues) // add samples to the power model
			}
			powers, err := r.GetPlatformPower(false)
			Expect(err).NotTo(HaveOccurred())
			Expect(len(powers)).Should(Equal(len(processFeatureValues)))
			// TODO: verify if the power makes sense
			Expect(powers[0]).Should(BeEquivalentTo(2500))
			idlePowers, err := r.GetPlatformPower(true)
			Expect(err).NotTo(HaveOccurred())
			Expect(len(idlePowers)).Should(Equal(len(processFeatureValues)))
			Expect(idlePowers[0]).Should(BeEquivalentTo(2000))
		})

		It("Get Process Components Power By Default Regression Estimator with ModelServerEndpoint", func() {
			testServer := httptest.NewServer(DummyWeightHandler)
			modelWeightFilepath := config.GetDefaultPowerModelURL(types.DynPower.String(), types.ComponentEnergySource)
			r := genRegressor(types.DynPower, types.ComponentEnergySource, testServer.URL, "", modelWeightFilepath, "")
			err := r.Start()
			Expect(err).To(BeNil())
			r.ResetSampleIdx()
			for _, processFeatureValues := range processFeatureValues {
				r.AddProcessFeatureValues(processFeatureValues) // add samples to the power model
			}
			compPowers, err := r.GetComponentsPower(false)
			Expect(err).NotTo(HaveOccurred())
			Expect(len(compPowers)).Should(Equal(len(processFeatureValues)))
			// TODO: verify if the power makes sense
			Expect(compPowers[0].Core).Should(BeEquivalentTo(2500))

			idlePowers, err := r.GetComponentsPower(true)
			Expect(err).NotTo(HaveOccurred())
			Expect(len(idlePowers)).Should(Equal(len(processFeatureValues)))
			Expect(idlePowers[0].Core).Should(BeEquivalentTo(2000))
		})
	})

	Context("without model server", func() {
		It("Get Node Platform Power By Default Regression Estimator without ModelServerEndpoint", func() {
			/// Estimate Node Components Power using Linear Regression
			modelWeightFilepath := config.GetDefaultPowerModelURL(types.AbsPower.String(), types.ComponentEnergySource)
			initModelURL := "https://raw.githubusercontent.com/sustainable-computing-io/kepler-model-db/main/models/v0.6/nx12/std_v0.6/acpi/AbsPower/BPFOnly/SGDRegressorTrainer_1.json"
			r := genRegressor(types.AbsPower, types.PlatformEnergySource, "", initModelURL, modelWeightFilepath, "")
			err := r.Start()
			Expect(err).To(BeNil())
			r.ResetSampleIdx()
			r.AddNodeFeatureValues(nodeFeatureValues) // add samples to the power model
			_, err = r.GetComponentsPower(false)
			Expect(err).NotTo(HaveOccurred())
		})

		It("Get Node Components Power By Default Regression Estimator without ModelServerEndpoint", func() {
			/// Estimate Node Components Power using Linear Regression
			modelWeightFilepath := config.GetDefaultPowerModelURL(types.AbsPower.String(), types.ComponentEnergySource)
			initModelURL := "https://raw.githubusercontent.com/sustainable-computing-io/kepler-model-db/main/models/v0.6/nx12/std_v0.6/rapl/AbsPower/BPFOnly/SGDRegressorTrainer_1.json"
			r := genRegressor(types.AbsPower, types.ComponentEnergySource, "", initModelURL, modelWeightFilepath, "")
			err := r.Start()
			Expect(err).To(BeNil())
			r.ResetSampleIdx()
			r.AddNodeFeatureValues(nodeFeatureValues) // add samples to the power model
			_, err = r.GetComponentsPower(false)
			Expect(err).NotTo(HaveOccurred())
		})

		It("Get Process Components Power By Default Regression Estimator without ModelServerEndpoint", func() {
			// Estimate Process Components Power using Linear Regression
			modelWeightFilepath := config.GetDefaultPowerModelURL(types.DynPower.String(), types.ComponentEnergySource)
			initModelURL := "https://raw.githubusercontent.com/sustainable-computing-io/kepler-model-db/main/models/v0.6/nx12/std_v0.6/acpi/DynPower/BPFOnly/SGDRegressorTrainer_1.json"
			r := genRegressor(types.DynPower, types.PlatformEnergySource, "", initModelURL, modelWeightFilepath, "")
			err := r.Start()
			Expect(err).To(BeNil())
			r.ResetSampleIdx()
			for _, processFeatureValues := range processFeatureValues {
				r.AddProcessFeatureValues(processFeatureValues) // add samples to the power model
			}
			_, err = r.GetComponentsPower(false)
			Expect(err).NotTo(HaveOccurred())
		})

		It("Get Process Components Power By Default Regression Estimator without ModelServerEndpoint", func() {
			// Estimate Process Components Power using Linear Regression
			modelWeightFilepath := config.GetDefaultPowerModelURL(types.DynPower.String(), types.ComponentEnergySource)
			initModelURL := "https://raw.githubusercontent.com/sustainable-computing-io/kepler-model-db/main/models/v0.6/nx12/std_v0.6/rapl/DynPower/BPFOnly/SGDRegressorTrainer_1.json"
			r := genRegressor(types.DynPower, types.ComponentEnergySource, "", initModelURL, modelWeightFilepath, "")
			err := r.Start()
			Expect(err).To(BeNil())
			r.ResetSampleIdx()
			for _, processFeatureValues := range processFeatureValues {
				r.AddProcessFeatureValues(processFeatureValues) // add samples to the power model
			}
			_, err = r.GetComponentsPower(false)
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Context("with core ratio", Ordered, func() {
		DescribeTable("Test core ratio computation", func(discoveredCore, modelCores int, expectedCoreRatio float64) {
			ModelCores = modelCores
			testServer := httptest.NewServer(DummyWeightHandler)
			modelWeightFilepath := config.GetDefaultPowerModelURL(types.DynPower.String(), types.PlatformEnergySource)
			r := genRegressor(types.DynPower, types.PlatformEnergySource, testServer.URL, "", modelWeightFilepath, "")
			r.DiscoveredMachineSpec = &config.MachineSpec{
				Cores: discoveredCore,
			}
			err := r.Start()
			Expect(err).To(BeNil())
			r.ResetSampleIdx()
			for _, processFeatureValues := range processFeatureValues {
				r.AddProcessFeatureValues(processFeatureValues) // add samples to the power model
			}
			powers, err := r.GetPlatformPower(false)
			Expect(err).NotTo(HaveOccurred())
			Expect(len(powers)).Should(Equal(len(processFeatureValues)))
			// TODO: verify if the power makes sense
			Expect(powers[0]).Should(BeEquivalentTo(ExpectedAbsPowerFromDummyWeightHandler))
			idlePowers, err := r.GetPlatformPower(true)
			Expect(err).NotTo(HaveOccurred())
			Expect(len(idlePowers)).Should(Equal(len(processFeatureValues)))
			expectedIdlePower := uint64(float64(ExpectedIdlePowerFromDummyWeightHandler) * expectedCoreRatio)
			Expect(idlePowers[0]).Should(BeEquivalentTo(expectedIdlePower))
		},
			Entry("equal core", 16, 16, 1.0),
			Entry("VM core ratio 0.25)", 4, 16, 0.25),
			Entry("VM core ratio 4)", 16, 4, 1.0),
			Entry("invalid discovered core", 0, 16, 1.0),
			Entry("invalid model core", 16, 0, 1.0),
		)
	})

	DescribeTable("Test ComponentModelWeights.Trainer", func(modelName, expectedTrainer string) {
		w := ComponentModelWeights{
			ModelName: modelName,
		}
		Expect(w.Trainer()).To(Equal(expectedTrainer))
	},
		Entry("empty model name", "", ""),
		Entry("invalid model name", "some", ""),
		Entry("invalid model name with _", "some_invalid", ""),
		Entry("valid SGDRegressorTrainer", "SGDRegressorTrainer_0", "SGDRegressorTrainer"),
		Entry("valid LogarithmicRegressionTrainer", "LogarithmicRegressionTrainer_0", "LogarithmicRegressionTrainer"),
		Entry("valid LogisticRegressionTrainer", "LogisticRegressionTrainer_0", "LogisticRegressionTrainer"),
		Entry("valid ExponentialRegressionTrainer", "ExponentialRegressionTrainer_0", "ExponentialRegressionTrainer"),
		Entry("invalid GradientBoostingRegressorTrainer", "GradientBoostingRegressorTrainer_0", ""),
	)
})
