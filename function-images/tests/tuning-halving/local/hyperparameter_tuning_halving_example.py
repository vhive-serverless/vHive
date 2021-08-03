import sklearn.datasets as datasets
from sklearn.ensemble import RandomForestRegressor
from sklearn.model_selection import  cross_val_predict
from sklearn.metrics import roc_auc_score
import itertools
import numpy as np
import pickle
from sklearn.model_selection import StratifiedShuffleSplit


def fake_write_to_s3(obj, key):
	pickle.dump(obj, open(key, "wb" ))


def fake_read_from_s3(key):
	return pickle.load(open(key, "rb" ))


def generate_dataset():
	n_samples = 1000
	n_features = 1024 
	X, y = datasets.make_classification(n_samples,
	                                    n_features,
	                                    n_redundant=0,
	                                    n_clusters_per_class=2,
	                                    weights=[0.9, 0.1],
	                                    flip_y=0.1,
	                                    random_state=42)
	return {'features': X, 'labels': y}


def generate_hyperparam_sets(param_config):
	keys = list(param_config.keys())
	values = [param_config[k] for k in keys]

	for elements in itertools.product(*values):
		yield dict(zip(keys, elements))


def handler_broker(event, context):
	dataset = generate_dataset()
	hyperparam_config = {
		'model': 'RandomForestRegressor',
		'params': {
			'n_estimators': [5, 10, 20],
			'min_samples_split': [2, 4],
			'random_state': [42]
		}
	}
	models_config = {
		'models': [
			{
				'model': 'RandomForestRegressor',
				'params': hyperparam
			} for hyperparam in generate_hyperparam_sets(hyperparam_config['params'])
		]
	}
	fake_write_to_s3(dataset, 'dataset_key')
	return {
		'dataset_key': 'dataset_key',
		'models_config': models_config
	}


def handler_model_training(event, context):
	# Read from S3
	dataset = fake_read_from_s3(event['dataset_key'])
	model_config = event['model_config']
	sample_rate = event['sample_rate']
	
	# Init model
	model_class = model_dispatcher(model_config['model'])
	model = model_class(**model_config['params'])

	# Train model and get predictions
	X = dataset['features']
	y = dataset['labels']
	if sample_rate==1.0:
		X_sampled, y_sampled = X, y
	else:
		stratified_split = StratifiedShuffleSplit(n_splits=1, train_size=sample_rate, random_state=42)
		sampled_index, _ = list(stratified_split.split(X, y))[0]
		X_sampled, y_sampled = X[sampled_index], y[sampled_index]
	y_pred = cross_val_predict(model, X_sampled, y_sampled, cv=5)
	model.fit(X_sampled, y_sampled)
	score = roc_auc_score(y_sampled, y_pred)
	print(f"{model_config['model']}, params: {model_config['params']}, dataset size: {len(y_sampled)},score: {score}")

	# Write to S3
	model_key = f"model_{event['count']}"
	pred_key = f"pred_model_{event['count']}"
	fake_write_to_s3(model, model_key)
	fake_write_to_s3(y_pred, pred_key)
	return {
		'model_key': model_key,
		'pred_key': pred_key,
		'score': score,
		'params': model_config
	}


# Flow which uses handlers
def orchestrator_flow():
	event = handler_broker({}, {})
	models = event['models_config']['models']

	while len(models)>1:
		sample_rate = 1/len(models)
		print(f"Running {len(models)} models on the dataset with sample rate {sample_rate} ")
		# Run different model configs on sampled dataset
		training_responses = []
		for count, model_config in enumerate(models):
			training_responses.append(
				handler_model_training({
					'dataset_key': event['dataset_key'],
					'model_config': model_config,
					'count': count,
					'sample_rate': sample_rate
				}, {})
			)

		# Keep models with the best score
		top_number = len(training_responses)//2
		sorted_responses = sorted(training_responses, key=lambda result: result['score'], reverse=True)
		models = [resp['params'] for resp in sorted_responses[:top_number]]

	print(f"Training final model {models[0]} on the full dataset")
	final_response = handler_model_training({
		'dataset_key': event['dataset_key'],
		'model_config': models[0],
		'count': 0,
		'sample_rate': 1.0
	}, {})
	return final_response


def model_dispatcher(model_name):
	if model_name=='LinearSVR':
		return LinearSVR
	elif model_name=='LinearRegression':
		return LinearRegression
	elif model_name=='RandomForestRegressor':
		return RandomForestRegressor
	elif model_name=='KNeighborsRegressor':
		return KNeighborsRegressor
	elif model_name=='LogisticRegression':
		return LogisticRegression
	else:
		raise ValueError(f"Model {model_name} not found") 


def main():
	orchestrator_flow()


if __name__ == '__main__':
	main()