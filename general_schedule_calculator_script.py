#!/usr/bin/env python3
"""
CronJob Schedule Calculator for OpenShift

This script fetches CronJob schedules from OpenShift clusters and calculates
the next N scheduled runs.

Usage:
    python general_schedule_calculator_script.py -n 5
"""

import subprocess
import argparse
import sys
import logging
from datetime import datetime, timezone
from cron_converter import Cron

logging.basicConfig(
    level=logging.INFO,
    format='%(asctime)s %(levelname)s %(message)s',
    datefmt='%Y-%m-%d %H:%M:%S'
)
logger = logging.getLogger(__name__)

CRONJOB_NAME = "create-dependencyupdatecheck"
CRONJOB_NAMESPACE = "mintmaker"

def get_cronjob_schedule_from_oc(cronjob_name, namespace=CRONJOB_NAMESPACE):
    try:
        result = subprocess.run([
            'oc', 'get', 'cronjob', cronjob_name, 
            '-n', namespace, 
            '-o', 'jsonpath={.spec.schedule}'
        ], capture_output=True, text=True, check=True)
        
        schedule = result.stdout.strip()
        logger.info("Found cronjob schedule: %s.", schedule)
        return schedule
        
    except Exception as e:
        logger.error("Error fetching cronjob schedule: %s.", e)
        return None


def analyze_cron_schedule(cron_expression, number_of_runs):
    logger.info("Analyzing CronJob schedule for next %d runs.", number_of_runs)
    cron_instance = Cron()
    cron_instance.from_string(cron_expression)

    reference = datetime.now(timezone.utc)
    schedule = cron_instance.schedule(reference)
    
    next_runs = []
    for _ in range(number_of_runs):
        next_runs.append(schedule.next().strftime('%Y-%m-%d %H:%M:%S %Z'))
    
    return next_runs


def write_to_csv(next_runs, filename="scheduled_times.csv"):
    try:
        with open(filename, 'w', encoding='utf-8') as output_file:
            for time in next_runs:
                output_file.write(f"{time}\n")
        logger.info("Results written to %s.", filename)
        return True
        
    except Exception as e:
        logger.error("Could not write to file %s: %s.", filename, e)
        return False


def main():
    try:
        parser = argparse.ArgumentParser(description='Analyze CronJob schedules from OpenShift')
        parser.add_argument('-n', '--count', type=int, default=5, 
                            help='Number of next scheduled runs to calculate (default: 5)')
        args = parser.parse_args()
        
        general_schedule = get_cronjob_schedule_from_oc(CRONJOB_NAME)
        if not general_schedule:
            return 1
            
        result = analyze_cron_schedule(general_schedule, args.count)
        
        if not write_to_csv(result):
            return 1
            
        logger.info("Schedule analysis complete.")
        return 0

    except Exception as e:
        logger.error("Error while analyzing cronjob schedule: %s.", e)
        return 1

if __name__ == "__main__":
    sys.exit(main())

