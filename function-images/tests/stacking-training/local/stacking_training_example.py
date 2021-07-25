import sklearn.datasets as datasets
from sklearn.linear_model import LogisticRegression
from sklearn.neighbors import KNeighborsRegressor
from sklearn.ensemble import RandomForestRegressor
from sklearn.svm import LinearSVR
from sklearn.linear_model import LinearRegression, Lasso
from sklearn.model_selection import  cross_val_predict
from sklearn.metrics import roc_auc_score
import numpy as np
import pickle


from timeit import default_timer as now

def fake_write_to_s3(obj, key):
    pickle.dump(obj, open(key, "wb" ))


def fake_read_from_s3(key):
    return pickle.load(open(key, "rb" ))


def generate_dataset():
    n_samples = 300
    n_features = 1024 
    X, y = datasets.make_classification(n_samples,
                                        n_features,
                                        n_redundant=0,
                                        n_clusters_per_class=2,
                                        weights=[0.9, 0.1],
                                        flip_y=0.1,
                                        random_state=42)
    return {'features': X, 'labels': y}


def handler_broker(event, context):
    dataset = generate_dataset()
    model_config = {
        'models': [
            {
                'model': 'LinearSVR',
                'params': {
                    'C': 1.0,
                    'tol': 1e-6,
                    'random_state': 42
                }
            },
            {
                'model': 'Lasso',
                'params': {
                    'alpha': 0.1
                }
            },
            {
                'model': 'RandomForestRegressor',
                'params': {
                    'n_estimators': 2,
                    'max_depth': 2,
                    'min_samples_split': 2,
                    'min_samples_leaf': 2,
                    #'n_jobs': 2,
                    'random_state': 42
                }
            },
            {
                'model': 'KNeighborsRegressor',
                'params': {
                    'n_neighbors': 20,
                }    
            }
        ],
        'meta_model': {
            'model': 'LogisticRegression',
            'params': {}
        }
    }
    fake_write_to_s3(dataset, 'dataset_key')
    return {
        'dataset_key': 'dataset_key',
        'model_config': model_config
    }


def handler_model_training(event, context):
    # Read from S3
    dataset = fake_read_from_s3(event['dataset_key'])
    model_config = event['model_config']
    
    # Init model
    model_class = model_dispatcher(model_config['model'])
    model = model_class(**model_config['params'])

    # Train model and get predictions
    y_pred = cross_val_predict(model, dataset['features'], dataset['labels'], cv=5)
    model.fit(dataset['features'], dataset['labels'])
    print(f"{model_config['model']} score: {roc_auc_score(dataset['labels'], y_pred)}")

    # Write to S3
    model_key = f"model_{event['count']}"
    pred_key = f"pred_model_{event['count']}"
    fake_write_to_s3(model, model_key)
    fake_write_to_s3(y_pred, pred_key)
    return {
        'model_key': model_key,
        'pred_key': pred_key
    }


def handler_reducer(event, context):
    # Aggregate and read from S3
    models = []
    predictions = []
    for training_response in event:
        models.append(fake_read_from_s3(training_response['model_key']))
        predictions.append(fake_read_from_s3(training_response['pred_key']))
    meta_features = np.transpose(np.array(predictions))

    # Write to S3
    meta_features_key = 'meta_features_key'
    models_key = 'models_key'
    fake_write_to_s3(meta_features, meta_features_key)
    fake_write_to_s3(models, models_key)
    return {
        'meta_features_key': meta_features_key,
        'models_key': models_key
    }


def handler_meta_model_training(event, context):
    # Read from S3
    dataset = fake_read_from_s3(event['dataset_key'])
    meta_features = fake_read_from_s3(event['meta_features_key'])
    models = fake_read_from_s3(event['models_key'])

    # Init meta model
    model_config = event['model_config']['meta_model']
    model_class = model_dispatcher(model_config['model'])
    meta_model = model_class(*model_config['params'])

    # Train meta model and get predictions
    meta_predictions = cross_val_predict(meta_model, meta_features, dataset['labels'], cv=5)
    score = roc_auc_score(meta_predictions, dataset['labels'])
    print(f"Ensemble model score {score}")
    meta_model.fit(meta_features, dataset['labels'])
    model_full = {
        'models': models,
        'meta_model': meta_model
    }

    # Write to S3
    model_full_key = 'model_full_key'
    meta_predictions_key = 'meta_predictions_key'
    fake_write_to_s3(meta_predictions, meta_predictions_key)
    fake_write_to_s3(model_full, model_full_key)

    # Assemble final response
    return {
        'model_full_key': model_full_key,
        'score': score,
        'predictions_key': meta_predictions_key
    }


# Flow which uses handlers
def orchestrator_flow():
    ts_start = now()
    event = handler_broker({}, {})
    ts_end = now()
    ts = (ts_end - ts_start) * 1000
    print(">>>>>>broker {:.0f}ms".format(ts))

    training_responses = []
    for count, model_config in enumerate(event['model_config']['models']):
        ts_start = now()
        training_responses.append(
            handler_model_training({
                'dataset_key': event['dataset_key'],
                'model_config': model_config,
                'count': count
            }, {})
        )
        ts = (now() - ts_start) * 1000
        print(">>>>>>training-{} {:.0f}ms".format(count, ts))
    
    ts_start = now()
    reducer_response = handler_reducer(training_responses, {})
    ts = (now() - ts_start) * 1000
    print(">>>>>>reducer {:.0f}ms".format(ts))
    
    ts_start = now()
    reducer_response['dataset_key'] = event['dataset_key']
    reducer_response['model_config'] = event['model_config']
    final_response = handler_meta_model_training(reducer_response, {})
    ts = (now() - ts_start) * 1000
    print(">>>>>>meta_training {:.0f}ms".format(ts))
    return final_response


def model_dispatcher(model_name):
    if model_name=='LinearSVR':
        return LinearSVR
    elif model_name=='Lasso':
        return Lasso
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


# Single flow implementation for comparison
def single_flow_pipeline():
    dataset = generate_dataset()
    models = [
        LinearSVR(C=1.0, random_state=42),
        LinearRegression(),
        RandomForestRegressor(n_estimators=10, random_state=42),
        KNeighborsRegressor(n_neighbors=5)
    ]
    meta_model = LogisticRegression()

    predictions = []
    for model in models:
        y_pred = cross_val_predict(model, dataset['features'], dataset['labels'], cv=5)
        model.fit(dataset['features'], dataset['labels'])
        predictions.append(y_pred)
        print(roc_auc_score(dataset['labels'], y_pred))

    meta_features = np.transpose(np.array(predictions))
    meta_predictions = cross_val_predict(meta_model, meta_features, dataset['labels'], cv=5)
    score = roc_auc_score(meta_predictions, dataset['labels'])
    print(f"Ensemble model score {score}")
    meta_model.fit(meta_features, dataset['labels'])
    return {
        'model': {
            'models': models,
            'meta_model': meta_model
        },
        'score': score,
        'predictions': meta_predictions
    }


def main():
    # single_flow_pipeline()
    orchestrator_flow()


if __name__ == '__main__':
    main()
